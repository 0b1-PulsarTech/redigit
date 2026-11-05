package lsp

import (
	"encoding/json"
	"testing"
)

func TestInitializeParamsMarshal(t *testing.T) {
	t.Parallel()

	params := InitializeParams{
		ProcessID: 42,
		RootURI:   "file:///repo",
		Capabilities: ClientCapabilities{
			General: GeneralClientCapabilities{PositionEncodings: []string{"utf-16"}},
			TextDocument: TextDocumentClientCapabilities{
				DocumentSymbol: DocumentSymbolClientCapabilities{
					HierarchicalDocumentSymbolSupport: true,
				},
			},
		},
		InitializationOptions: map[string]any{"buildFlags": []string{"-tags=integration"}},
	}

	body, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	opts, ok := decoded["initializationOptions"].(map[string]any)
	if !ok {
		t.Fatalf(
			"initializationOptions missing or wrong type: %v",
			decoded["initializationOptions"],
		)
	}

	if _, ok := opts["buildFlags"]; !ok {
		t.Fatalf("initializationOptions has no flat buildFlags key: %v", opts)
	}

	if _, nested := opts["build"]; nested {
		t.Fatalf("initializationOptions nested a build object instead of a flat key: %v", opts)
	}
}

func TestDocumentSymbolUnmarshal(t *testing.T) {
	t.Parallel()

	body := `{
		"name": "Dog",
		"kind": 23,
		"range": {"start":{"line":0,"character":0},"end":{"line":10,"character":1}},
		"selectionRange": {"start":{"line":0,"character":5},"end":{"line":0,"character":8}},
		"children": [
			{
				"name": "Name",
				"kind": 8,
				"range": {"start":{"line":1,"character":1},"end":{"line":1,"character":10}},
				"selectionRange": {"start":{"line":1,"character":1},"end":{"line":1,"character":5}}
			}
		]
	}`

	var symbol DocumentSymbol
	if err := json.Unmarshal([]byte(body), &symbol); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if symbol.Name != "Dog" || symbol.Kind != SymbolKindStruct {
		t.Fatalf(
			"got name=%q kind=%d, want name=Dog kind=%d",
			symbol.Name,
			symbol.Kind,
			SymbolKindStruct,
		)
	}

	if len(symbol.Children) != 1 || symbol.Children[0].Name != "Name" {
		t.Fatalf("children = %+v, want one field named Name", symbol.Children)
	}
}

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
