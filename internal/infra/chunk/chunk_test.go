package chunk

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// assertReconstructs is the coverage invariant every chunker in this package
// must satisfy: concatenating the given decls' spans in order must reproduce
// source byte for byte, with no overlap and no gap.
func assertReconstructs(t *testing.T, source string, decls []entities.Decl) {
	t.Helper()

	cursor := 0

	for _, decl := range decls {
		if decl.Span.Start != cursor {
			t.Fatalf(
				"decl %q span %v does not start where the previous one ended (%d)",
				decl.Key.Name, decl.Span, cursor,
			)
		}

		cursor = decl.Span.End
	}

	if cursor != len(source) {
		t.Fatalf("decls cover [0,%d), want [0,%d)", cursor, len(source))
	}
}

// assertDeclNames checks that decls carries exactly the given names, in
// order - the shape every Chunker test in this package needs, whether the
// source was a query file, a go.mod, or a go.work.
func assertDeclNames(t *testing.T, decls []entities.Decl, wantNames []string) {
	t.Helper()

	if len(decls) != len(wantNames) {
		t.Fatalf("got %d decls, want %d (%+v)", len(decls), len(wantNames), decls)
	}

	for i, decl := range decls {
		if decl.Key.Name != wantNames[i] {
			t.Fatalf("decl[%d].Key.Name = %q, want %q", i, decl.Key.Name, wantNames[i])
		}

		if decl.Key.Kind != entities.KindOther {
			t.Fatalf("decl[%d].Key.Kind = %q, want %q", i, decl.Key.Kind, entities.KindOther)
		}
	}
}
