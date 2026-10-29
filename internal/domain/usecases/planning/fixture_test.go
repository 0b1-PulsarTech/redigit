package planning

import (
	"fmt"
	"strconv"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func proseEdge(dependent, provider entities.Decl) entities.Edge {
	return entities.Edge{
		Dependent: dependent.Key,
		Provider:  provider.Key,
		Reason:    entities.ReasonProse,
	}
}

func keysOf(decls []entities.Decl) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(decls))
	for _, item := range decls {
		out = append(out, item.Key)
	}

	return out
}

// rooted is twoFeatures plus a composition root that wires both handlers and
// enough registrations of its own to read as the program's assembly.
func rooted() (entities.Graph, entities.Ordering, map[string]entities.Decl) {
	base, order, named := twoFeatures()
	root := decl("cmd/app/main.go", "Run", "cmd/app")

	decls := base.Decls()
	edges := base.Edges()
	at := map[entities.DeclKey]entities.Position{}

	for _, d := range decls {
		at[d.Key] = order.Of(d.Key)
	}

	decls = append(decls, root)
	edges = append(
		edges,
		needs(root, named["leadAPI"]),
		needs(root, named["billAPI"]),
	)
	at[root.Key] = entities.Position(len(at))

	for index := range 20 {
		ballast := decl("cmd/app/bind.go", fmt.Sprintf("Bind%d", index), "cmd/app")
		decls = append(decls, ballast)
		edges = append(edges, needs(root, ballast))
		at[ballast.Key] = entities.Position(len(at))
	}

	named["root"] = root

	return entities.NewGraph(decls, edges, nil), entities.Ordering{At: at}, named
}

func statement(function, file string, index int) entities.Decl {
	return entities.Decl{
		Key: entities.DeclKey{
			File: file,
			Kind: entities.KindStmt,
			Name: function + "#" + strconv.Itoa(index),
		},
		Unit: "cmd/app",
	}
}

func prose(name string) entities.Decl {
	found := decl("CHANGELOG.md", name, "")
	found.Key.Kind = entities.KindOther

	return found
}
