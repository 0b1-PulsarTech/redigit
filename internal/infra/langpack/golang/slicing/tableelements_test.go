package slicing

import (
	"context"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/render"
)

func TestSliceCutsAReturnedListByElement(t *testing.T) {
	t.Parallel()

	source := []byte(sliceSource)

	for _, name := range []string{"Table", "Chain"} {
		head, ok, members, _ := sliceOf(t, name)
		if !ok || len(members) != 2 {
			t.Fatalf("Slice(%s) = %v with %d members, want 2 elements", name, ok, len(members))
		}

		if head.Atom == "" || head.Atom != members[0].Atom || members[1].Atom != "" {
			t.Fatalf("atoms head %q first %q second %q, want head born with the first element",
				head.Atom, members[0].Atom, members[1].Atom)
		}

		got, err := render.Render(
			context.Background(), source, []entities.DeclKey{head.Key, members[1].Key},
		)
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}

		if strings.Contains(string(got), "runA") || strings.Contains(string(got), "tableA") {
			t.Fatalf("Render(%s) = %s, want only the kept element", name, got)
		}
	}
}
