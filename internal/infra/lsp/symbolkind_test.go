package lsp

import "testing"

func TestSymbolKindValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		kind SymbolKind
		want int
	}{
		{name: "file is first", kind: SymbolKindFile, want: 1},
		{name: "struct matches spec", kind: SymbolKindStruct, want: 23},
		{name: "type parameter is last", kind: SymbolKindTypeParameter, want: 26},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if int(testCase.kind) != testCase.want {
				t.Fatalf("got = %d, want %d", testCase.kind, testCase.want)
			}
		})
	}
}
