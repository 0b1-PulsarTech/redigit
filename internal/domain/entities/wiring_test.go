package entities

import "testing"

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
