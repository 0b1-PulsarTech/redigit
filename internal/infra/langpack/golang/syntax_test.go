package golang

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const composedSource = `package app

func Register(srv *Server) {
	srv.Add(one())
	srv.Add(two())
}

func Single() {
	run()
}
`

func TestSyntaxComposesDeclarationsRenderAndSlice(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	source := []byte(composedSource)
	syntax := NewSyntax()

	decls, err := syntax.Declarations(ctx, "app/app.go", source)
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	if len(decls) != 2 {
		t.Fatalf("Declarations() = %d declarations, want 2", len(decls))
	}

	keep := []entities.DeclKey{decls[1].Key}

	rendered, err := syntax.Render(ctx, source, keep)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if want := "package app\n\nfunc Single() {\n\trun()\n}\n"; string(rendered) != want {
		t.Fatalf("Render() = %q, want %q", rendered, want)
	}

	testCases := []struct {
		name       string
		decl       entities.Decl
		wantSliced bool
	}{
		{name: "a body of two statements is cut", decl: decls[0], wantSliced: true},
		{name: "a body of one statement is left whole", decl: decls[1], wantSliced: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, sliced, err := syntax.Slice(ctx, testCase.decl, source)
			if err != nil {
				t.Fatalf("Slice() error = %v", err)
			}

			if sliced != testCase.wantSliced {
				t.Fatalf("Slice() sliced = %v, want %v", sliced, testCase.wantSliced)
			}
		})
	}
}

func TestSyntaxPassesErrorsThrough(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	syntax := NewSyntax()

	if _, err := syntax.Declarations(ctx, "bad.go", []byte("package")); err == nil {
		t.Fatal("Declarations() error = nil, want a parse error")
	}

	if _, err := syntax.Render(
		ctx,
		[]byte("package"),
		[]entities.DeclKey{{Name: "x"}},
	); err == nil {
		t.Fatal("Render() error = nil, want a parse error")
	}

	fn := entities.Decl{Key: entities.DeclKey{File: "bad.go", Kind: entities.KindFunc, Name: "f"}}
	if _, _, err := syntax.Slice(ctx, fn, []byte("package")); err == nil {
		t.Fatal("Slice() error = nil, want a parse error")
	}
}

func TestSyntaxCarriesImportsThroughTheRenderPackage(t *testing.T) {
	t.Parallel()

	syntax := NewSyntax()
	source := []byte("package p\n\nimport \"bytes\"\n\nfunc F() {}\n")

	if got := syntax.Needed(source, "func G() { _ = bytes.Equal(nil, nil) }"); len(got) != 1 {
		t.Fatalf("Needed() = %q, want the bytes import", got)
	}

	admitted, err := syntax.Admit(context.Background(), source, []string{`"os"`})
	if err != nil || string(admitted) == string(source) {
		t.Fatalf("Admit() = %q, %v, want the os import added", admitted, err)
	}

	if _, err = syntax.Admit(context.Background(), source, []string{"=="}); err == nil {
		t.Fatalf("Admit() error = nil, want a spec that does not parse refused")
	}
}
