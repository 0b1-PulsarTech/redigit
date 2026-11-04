package e2e

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

type chainSet struct {
	rootA, rootB, zed, orders, billingA, billingB, users, cycleA, cycleB entities.DeclKey
	billing                                                              entities.DeclKey
	nested, other                                                        entities.DeclKey
	known                                                                world
	decls                                                                []entities.Decl
}

func fileKey(file string, kind entities.DeclKind, name string) entities.DeclKey {
	return entities.DeclKey{File: file, Kind: kind, Name: name}
}

func newChainSet() chainSet {
	var (
		router = facts.TypeRef{Name: "m/router.Router"}
		set    = chainSet{
			rootA:    fileKey("app/routes.go", entities.KindStmt, "Routes#1"),
			rootB:    fileKey("app/routes.go", entities.KindStmt, "Routes#2"),
			zed:      fileKey("zed/zed.go", entities.KindFunc, "Mount"),
			orders:   fileKey("orders/orders.go", entities.KindFunc, "Mount"),
			billingA: fileKey("billing/billing.go", entities.KindStmt, "Mount#1"),
			billing:  fileKey("billing/billing.go", entities.KindFunc, "Mount"),
			billingB: fileKey("billing/billing.go", entities.KindStmt, "Mount#2"),
			users:    fileKey("users/users.go", entities.KindFunc, "Mount"),
			cycleA:   fileKey("loop/a.go", entities.KindFunc, "Mount"),
			cycleB:   fileKey("loop/b.go", entities.KindFunc, "Mount"),
			nested:   fileKey("app/routes.go", entities.KindStmt, "Routes#3"),
			other:    fileKey("cmd/srv/other.go", entities.KindStmt, "Setup#1"),
		}
		get = func(site entities.DeclKey, path string) facts.Call {
			return facts.Call{
				Site: site, Callee: "m/router.Router.GET", Receiver: router,
				Literals: []string{path},
			}
		}
		mount = func(site, target entities.DeclKey, literals ...string) facts.Call {
			return facts.Call{
				Site: site, Callee: "m/router.Router.Mount", Receiver: router,
				FuncArgs: []entities.DeclKey{target}, Literals: literals,
			}
		}
		call = func(site, target entities.DeclKey) facts.Call {
			return facts.Call{Site: site, Callee: "m/pkg.Mount", CalleeKey: target}
		}
		scope = facts.Call{
			Site: set.nested, Callee: "m/router.Router.Route", Receiver: router,
			Literals: []string{"/v1"},
		}
	)

	nestedCall := call(set.nested, set.users)
	nestedCall.Within = &scope

	set.known = world{
		set.rootA:    {call(set.rootA, set.zed)},
		set.rootB:    {mount(set.rootB, set.billing, "/billing")},
		set.nested:   {nestedCall},
		set.other:    {call(set.other, set.users)},
		set.zed:      {call(set.zed, set.orders), get(set.zed, "/zedping")},
		set.orders:   {get(set.orders, "/orders"), call(set.orders, set.users)},
		set.billingA: {get(set.billingA, "/invoices")},
		set.billingB: {get(set.billingB, "/receipts")},
		set.users:    {get(set.users, "/users")},
		set.cycleA:   {call(set.cycleA, set.cycleB), get(set.cycleA, "/loopa")},
		set.cycleB:   {call(set.cycleB, set.cycleA), get(set.cycleB, "/loopb")},
	}

	for key := range set.known {
		set.decls = append(set.decls, entities.Decl{Key: key})
	}

	return set
}

func TestRuleEdgesWalkEveryCallerAndJoinTheirPrefix(t *testing.T) {
	t.Parallel()

	var (
		set     = newChainSet()
		deep    = suiteKey("test", "TestDeep")
		users   = suiteKey("test", "TestUsers")
		invoice = suiteKey("test", "TestInvoices")
		v1      = suiteKey("test", "TestV1")
		loop    = suiteKey("test", "TestLoop")
	)

	requireEdges(t, set.known, set.decls, []edgeCase{
		{
			name:     "a route two mounts deep waits for every site up and for no other Mount func",
			suite:    entities.Decl{Key: deep},
			literals: []string{"/orders"},
			want: []entities.Edge{
				edgeTo(deep, set.rootA), edgeTo(deep, set.orders), edgeTo(deep, set.zed),
			},
		},
		{
			name:     "a route mounted from several places waits for each chain",
			suite:    entities.Decl{Key: users},
			literals: []string{"/users"},
			want: []entities.Edge{
				edgeTo(users, set.rootA), edgeTo(users, set.other), edgeTo(users, set.orders),
				edgeTo(users, set.users), edgeTo(users, set.zed),
			},
		},
		{
			name:     "a constant prefix of the mount call is joined",
			suite:    entities.Decl{Key: invoice},
			literals: []string{"/billing/invoices"},
			want:     []entities.Edge{edgeTo(invoice, set.rootB), edgeTo(invoice, set.billingA)},
		},
		{
			name:     "a prefix of the call the mount sits in is joined",
			suite:    entities.Decl{Key: v1},
			literals: []string{"/v1/users"},
			want:     []entities.Edge{edgeTo(v1, set.nested), edgeTo(v1, set.users)},
		},
		{
			name:     "a path the mount prefix hides is not served bare",
			suite:    entities.Decl{Key: suiteKey("test", "TestBare")},
			literals: []string{"/invoices"},
			wantLog:  "no known route path",
		},
		{
			name:     "a mount cycle ends and still waits",
			suite:    entities.Decl{Key: loop},
			literals: []string{"/loopa"},
			want:     []entities.Edge{edgeTo(loop, set.cycleA), edgeTo(loop, set.cycleB)},
		},
	})
}
