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
