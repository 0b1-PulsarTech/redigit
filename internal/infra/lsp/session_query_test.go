package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSession_References(t *testing.T) {
	t.Parallel()

	session, server := fakeSession(t)
	resultCh := make(chan error, 1)

	var got []entities.Location

	at := entities.Location{File: "/repo/main.go", Line: 3, Char: 7}

	go func() {
		var err error
		got, err = session.References(context.Background(), at)
		resultCh <- err
	}()

	req := server.next()
	if req.Method != "textDocument/references" {
		t.Fatalf("method = %q, want textDocument/references", req.Method)
	}

	var params ReferenceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}

	if params.TextDocument.URI != "file:///repo/main.go" ||
		params.Position != (Position{Line: 3, Character: 7}) {
		t.Fatalf("params = %+v, want uri file:///repo/main.go position {3 7}", params)
	}

	if params.Context.IncludeDeclaration {
		t.Fatalf("IncludeDeclaration = true, want false")
	}

	server.respond(req.ID, []Location{
		{URI: "file:///repo/caller.go", Range: Range{Start: Position{Line: 10, Character: 2}}},
	})

	if err := <-resultCh; err != nil {
		t.Fatalf("References: %v", err)
	}

	want := entities.Location{File: "/repo/caller.go", Line: 10, Char: 2}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got = %+v, want [%+v]", got, want)
	}
}

func TestSession_References_InvalidPath(t *testing.T) {
	t.Parallel()

	session, _ := fakeSession(t)

	_, err := session.References(context.Background(), entities.Location{File: "relative.go"})
	if !errors.Is(err, ErrRelativePath) {
		t.Fatalf("err = %v, want ErrRelativePath", err)
	}
}

func TestSession_Implementations(t *testing.T) {
	t.Parallel()

	session, server := fakeSession(t)
	resultCh := make(chan error, 1)

	var got []entities.Location

	go func() {
		var err error
		got, err = session.Implementations(
			context.Background(),
			entities.Location{File: "/repo/iface.go", Line: 1, Char: 1},
		)
		resultCh <- err
	}()

	req := server.next()
	if req.Method != "textDocument/implementation" {
		t.Fatalf("method = %q, want textDocument/implementation", req.Method)
	}

	server.respond(
		req.ID,
		[]Location{
			{URI: "file:///repo/impl.go", Range: Range{Start: Position{Line: 5, Character: 0}}},
		},
	)

	if err := <-resultCh; err != nil {
		t.Fatalf("Implementations: %v", err)
	}

	want := entities.Location{File: "/repo/impl.go", Line: 5, Char: 0}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got = %+v, want [%+v]", got, want)
	}
}

func TestSession_Definition(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		result  locations
		want    entities.Location
		wantErr error
	}{
		{
			name: "single location",
			result: locations{
				{URI: "file:///repo/def.go", Range: Range{Start: Position{Line: 4, Character: 2}}},
			},
			want: entities.Location{File: "/repo/def.go", Line: 4, Char: 2},
		},
		{
			name:    "empty result is no definition",
			result:  locations{},
			wantErr: ErrNoDefinition,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			session, server := fakeSession(t)
			resultCh := make(chan error, 1)

			var got entities.Location

			go func() {
				var err error
				got, err = session.Definition(
					context.Background(),
					entities.Location{File: "/repo/use.go", Line: 1, Char: 1},
				)
				resultCh <- err
			}()

			req := server.next()
			server.respond(req.ID, []Location(testCase.result))

			err := <-resultCh
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && got != testCase.want {
				t.Fatalf("got = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestSession_DocumentSymbols(t *testing.T) {
	t.Parallel()

	session, server := fakeSession(t)
	resultCh := make(chan error, 1)

	var got []DocumentSymbol

	go func() {
		var err error
		got, err = session.DocumentSymbols(context.Background(), "/repo/main.go")
		resultCh <- err
	}()

	req := server.next()
	if req.Method != "textDocument/documentSymbol" {
		t.Fatalf("method = %q, want textDocument/documentSymbol", req.Method)
	}

	server.respond(req.ID, []DocumentSymbol{{Name: "main", Kind: SymbolKindFunction}})

	if err := <-resultCh; err != nil {
		t.Fatalf("DocumentSymbols: %v", err)
	}

	if len(got) != 1 || got[0].Name != "main" {
		t.Fatalf("got = %+v, want one symbol named main", got)
	}
}
