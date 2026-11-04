package entities

import (
	"fmt"
	"testing"
)

func TestFindWiringTreatsACutFunctionAsWiring(t *testing.T) {
	t.Parallel()

	head := Decl{Key: DeclKey{File: "cmd/main.go", Kind: KindFunc, Name: "run"}}
	method := Decl{Key: DeclKey{File: "cmd/main.go", Kind: KindMethod, Name: "Server.Start"}}
	first := Decl{Key: DeclKey{File: "cmd/main.go", Kind: KindStmt, Name: "run#0"}}
	second := Decl{Key: DeclKey{File: "cmd/main.go", Kind: KindStmt, Name: "Server.Start#0"}}
	other := Decl{Key: DeclKey{File: "cmd/main.go", Kind: KindFunc, Name: "helper"}}

	graph := NewGraph([]Decl{head, method, first, second, other}, nil, nil)
	roots := FindWiring(graph)

	testCases := []struct {
		name string
		key  DeclKey
		want bool
	}{
		{"a statement is wiring by kind", first.Key, true},
		{"the signature of a cut function is wiring", head.Key, true},
		{"so is the signature of a cut method", method.Key, true},
		{"a function nothing cut is not", other.Key, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := roots[testCase.key]; got != testCase.want {
				t.Fatalf("wiring[%v] = %v, want %v", testCase.key, got, testCase.want)
			}
		})
	}
}

// The step type of a bootstrap is read by the bootstrap's own wiring and by
// nothing else; a type a feature also reads stays with the feature.
func TestFindWiringTakesInTheTypesOnlyTheWiringReads(t *testing.T) {
	t.Parallel()

	step := Decl{Key: DeclKey{File: "boot/steps.go", Kind: KindType, Name: "step"}}
	shared := Decl{Key: DeclKey{File: "boot/steps.go", Kind: KindType, Name: "shared"}}
	elsewhere := Decl{Key: DeclKey{File: "other/use.go", Kind: KindType, Name: "elsewhere"}}
	stmt := Decl{Key: DeclKey{File: "boot/steps.go", Kind: KindStmt, Name: "steps#0"}}
	handler := Decl{Key: DeclKey{File: "boot/handler.go", Kind: KindFunc, Name: "handle"}}

	reads := func(from, to Decl) Edge {
		return Edge{Dependent: from.Key, Provider: to.Key, Reason: ReasonReference}
	}

	graph := NewGraph(
		[]Decl{step, shared, elsewhere, stmt, handler},
		[]Edge{
			reads(stmt, step),
			reads(stmt, shared),
			reads(handler, shared),
			reads(stmt, elsewhere),
		},
		nil,
	)

	roots := FindWiring(graph)

	if !roots[step.Key] {
		t.Errorf("step is read only by the wiring beside it, want it wiring")
	}

	if roots[shared.Key] {
		t.Errorf("shared is read by a handler, want it left to its feature")
	}

	if roots[elsewhere.Key] {
		t.Errorf("elsewhere sits in another directory, want it left to its feature")
	}
}

// A function only the wiring reads, and that itself calls into the wiring, is a
// registration step of the assembly; one that calls nothing of it is a feature's.
func TestFindWiringTakesInTheRegistrationStepsOfTheWiring(t *testing.T) {
	t.Parallel()

	stmt := Decl{Key: DeclKey{File: "boot/wire.go", Kind: KindStmt, Name: "inject#0"}}
	build := Decl{Key: DeclKey{File: "boot/build.go", Kind: KindStmt, Name: "build#0"}}
	register := Decl{Key: DeclKey{File: "boot/register.go", Kind: KindFunc, Name: "register"}}
	plain := Decl{Key: DeclKey{File: "boot/plain.go", Kind: KindFunc, Name: "plain"}}
	shared := Decl{Key: DeclKey{File: "boot/shared.go", Kind: KindFunc, Name: "shared"}}
	handler := Decl{Key: DeclKey{File: "boot/handler.go", Kind: KindFunc, Name: "handle"}}

	reads := func(from, to Decl) Edge {
		return Edge{Dependent: from.Key, Provider: to.Key, Reason: ReasonReference}
	}

	graph := NewGraph(
		[]Decl{stmt, build, register, plain, shared, handler},
		[]Edge{
			reads(stmt, register),
			reads(register, build),
			reads(stmt, plain),
			reads(stmt, shared),
			reads(handler, shared),
			reads(handler, build),
		},
		nil,
	)

	roots := FindWiring(graph)

	testCases := []struct {
		name string
		key  DeclKey
		want bool
	}{
		{"a step the wiring alone reads and that calls the wiring", register.Key, true},
		{"a function that calls nothing of the wiring", plain.Key, false},
		{"a function a handler also reads", shared.Key, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := roots[testCase.key]; got != testCase.want {
				t.Fatalf("wiring[%v] = %v, want %v", testCase.key, got, testCase.want)
			}
		})
	}
}

func decl(file, name string, unit UnitID) Decl {
	return Decl{
		Key:  DeclKey{File: file, Kind: KindFunc, Name: name},
		Unit: unit,
	}
}

func needs(dependent, provider Decl) Edge {
	return Edge{Dependent: dependent.Key, Provider: provider.Key, Reason: ReasonReference}
}

func twoFeatures() (Graph, map[string]Decl) {
	kernel := decl("kernel/id.go", "NewID", "kernel")

	leadUse := decl("lead/usecases/create.go", "Create", "lead/usecases")
	leadRepo := decl("lead/repo/lead.go", "Insert", "lead/repo")
	leadAPI := decl("lead/api/handler.go", "Handle", "lead/api")

	billUse := decl("bill/usecases/charge.go", "Charge", "bill/usecases")
	billAPI := decl("bill/api/handler.go", "Handle", "bill/api")

	decls := []Decl{kernel, leadUse, leadRepo, leadAPI, billUse, billAPI}
	edges := []Edge{
		needs(leadAPI, leadUse),
		needs(leadUse, leadRepo),
		needs(leadRepo, kernel),
		needs(billAPI, billUse),
		needs(billUse, kernel),
	}

	return NewGraph(decls, edges, nil), map[string]Decl{
		"kernel": kernel, "leadUse": leadUse, "leadRepo": leadRepo,
		"leadAPI": leadAPI, "billUse": billUse, "billAPI": billAPI,
	}
}

func rooted() (Graph, map[string]Decl) {
	base, named := twoFeatures()
	root := decl("cmd/app/main.go", "Run", "cmd/app")

	decls := base.Decls()
	edges := base.Edges()

	decls = append(decls, root)
	edges = append(edges, needs(root, named["leadAPI"]), needs(root, named["billAPI"]))

	for index := range 20 {
		ballast := decl("cmd/app/bind.go", fmt.Sprintf("Bind%d", index), "cmd/app")
		decls = append(decls, ballast)
		edges = append(edges, needs(root, ballast))
	}

	named["root"] = root

	return NewGraph(decls, edges, nil), named
}

func TestFindWiring(t *testing.T) {
	t.Parallel()

	graph, named := rooted()
	roots := FindWiring(graph)

	testCases := []struct {
		name string
		decl Decl
		want bool
	}{
		{"the root needs a twentieth of the program", named["root"], true},
		{"a handler needs a few declarations", named["leadAPI"], false},
		{"a leaf needs nothing", named["kernel"], false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := roots[testCase.decl.Key]; got != testCase.want {
				t.Fatalf("wiring = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestFindWiringCountsOnlyWhatARootCalls(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		reason EdgeReason
		want   bool
	}{
		{"a root that references its callees assembles the program", ReasonReference, true},
		{"register sites needing each other's types are not calls", ReasonProvides, false},
		{"a resolve site waiting for registrations is not a call", ReasonRegistered, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := decl("boot/boot.go", "Setup", "boot")
			decls := append(make([]Decl, 0, 25), root)
			edges := make([]Edge, 0, 24)

			for index := range 24 {
				leaf := decl("app/app.go", fmt.Sprintf("New%d", index), "app")
				decls = append(decls, leaf)
				edges = append(edges, Edge{
					Dependent: root.Key, Provider: leaf.Key, Reason: testCase.reason,
				})
			}

			roots := FindWiring(NewGraph(decls, edges, nil))
			if got := roots[root.Key]; got != testCase.want {
				t.Fatalf("wiring[Setup] = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestFindWiringIgnoresASmallProgram(t *testing.T) {
	t.Parallel()

	graph, _ := twoFeatures()

	if roots := FindWiring(graph); len(roots) != 0 {
		t.Fatalf("wiring = %v, want none: the program is smaller than any root", roots)
	}
}
