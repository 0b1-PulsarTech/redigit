package typed

import (
	"context"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestTypedSourceAttributesUsesToTheDeclarationsAsked(t *testing.T) {
	t.Parallel()

	snap, decls := testkit.Fixture(t, map[string]string{
		"a.go": "package fixture\n\nfunc One() {}\n\nfunc Run() {\n\tOne()\n}\n",
	})

	source := NewSource(nil)

	whole, err := source.Facts(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	run := testkit.DeclKey("a.go", entities.KindFunc, "Run")
	statement := testkit.DeclKey("a.go", entities.KindStmt, "Run#0")
	one := testkit.DeclKey("a.go", entities.KindFunc, "One")

	if !slices.Contains(whole.Uses, Use{Dependent: run, Provider: one}) {
		t.Fatalf("Uses = %v, want Run to use One", whole.Uses)
	}

	cut := slices.Clone(decls)

	for index, decl := range cut {
		if decl.Key != run {
			continue
		}

		head := decl
		head.Span.End = head.Span.Start + len("func Run() {")
		body := entities.Decl{
			Key:  statement,
			Unit: decl.Unit,
			Span: entities.Span{Start: head.Span.End + 2, End: decl.Span.End - 2},
		}
		cut[index] = head
		cut = append(cut, body)

		break
	}

	got, err := source.Facts(context.Background(), snap, cut)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	if !slices.Contains(got.Uses, Use{Dependent: statement, Provider: one}) ||
		slices.Contains(got.Uses, Use{Dependent: run, Provider: one}) {
		t.Fatalf("Uses = %v, want the statement to use One and the head not", got.Uses)
	}
}
