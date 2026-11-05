package stmtread

import (
	"context"
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var errBroken = errors.New("broken read")

func TestReaderRefusesOrSkipsWhatItCannotRead(t *testing.T) {
	t.Parallel()

	spans := map[entities.DeclKey]entities.Span{
		{File: "x", Kind: entities.KindStmt, Name: "F#0"}: {Start: 0, End: 3},
	}

	testCases := []struct {
		name    string
		file    string
		source  string
		readErr error
		wantErr error
		isBad   bool
	}{
		{name: "a non-Go file is not read at all", file: "page.dart", readErr: errBroken},
		{
			name:    "a Go file that cannot be read",
			file:    "a.go",
			readErr: errBroken,
			wantErr: errBroken,
		},
		{name: "a Go file that does not parse", file: "a.go", source: "func (", isBad: true},
		{name: "a span holding no statement finds nothing", file: "a.go", source: "package a\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			syntax := Reader{read: func(context.Context, string) ([]byte, error) {
				return []byte(testCase.source), testCase.readErr
			}}

			got, err := syntax.Statements(context.Background(), testCase.file, spans)
			if testCase.wantErr != nil && !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Statements() err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && (err != nil) != testCase.isBad {
				t.Fatalf("Statements() err = %v, want a parse error: %v", err, testCase.isBad)
			}

			if len(got) != 0 {
				t.Fatalf("Statements() = %v, want nothing", got)
			}
		})
	}
}

func TestReaderExempt(t *testing.T) {
	t.Parallel()

	graph := entities.NewGraph([]entities.Decl{
		{
			Key:  entities.DeclKey{File: "cmd/a.go", Kind: entities.KindFunc, Name: "main"},
			Unit: "cmd",
		},
	}, nil, nil)
	syntax := New(graph, nil)

	testCases := []struct {
		name string
		decl entities.Decl
		want string
	}{
		{
			name: "main is a root",
			decl: entities.Decl{
				Key:  entities.DeclKey{File: "cmd/a.go", Kind: entities.KindFunc, Name: "main"},
				Unit: "cmd",
			},
			want: "root",
		},
		{
			name: "init is a root",
			decl: entities.Decl{
				Key:  entities.DeclKey{File: "p/a.go", Kind: entities.KindFunc, Name: "init"},
				Unit: "p",
			},
			want: "root",
		},
		{
			name: "exported in a library",
			decl: entities.Decl{
				Key:  entities.DeclKey{File: "p/a.go", Kind: entities.KindFunc, Name: "Serve"},
				Unit: "p",
			},
			want: "exported",
		},
		{
			name: "exported in package main is measured",
			decl: entities.Decl{
				Key:  entities.DeclKey{File: "cmd/a.go", Kind: entities.KindType, Name: "Server"},
				Unit: "cmd",
			},
		},
		{
			name: "unexported in a library is measured",
			decl: entities.Decl{
				Key:  entities.DeclKey{File: "p/a.go", Kind: entities.KindFunc, Name: "serve"},
				Unit: "p",
			},
		},
		{
			name: "a file of another language is not told apart",
			decl: entities.Decl{
				Key:  entities.DeclKey{File: "lib/a.dart", Kind: entities.KindFunc, Name: "serve"},
				Unit: "lib",
			},
			want: "not Go",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := syntax.Exempt(testCase.decl); got != testCase.want {
				t.Fatalf("Exempt() = %q, want %q", got, testCase.want)
			}
		})
	}
}
