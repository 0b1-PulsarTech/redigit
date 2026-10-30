package slicing

import (
	"context"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
)

func atomShape(sliced indexing.Sliced) []int {
	atoms := make([]string, 0, len(sliced.Members)+1)
	for _, member := range sliced.Members {
		atoms = append(atoms, member.Atom)
	}

	atoms = append(atoms, sliced.Head.Atom)

	out := make([]int, 0, len(atoms))

	for _, atom := range atoms {
		if atom == "" {
			out = append(out, -1)

			continue
		}

		out = append(out, slices.Index(atoms, atom))
	}

	return out
}

func TestSliceJoinsLocalsInSourceOrder(t *testing.T) {
	t.Parallel()

	const runs = 256

	testCases := []struct {
		name string
		body string
		want []int
	}{
		{
			"a declaration read by a later statement through another local joins it too",
			`decls, where, err := read()
	if err != nil {
		return err
	}
	use(decls)
	use(decls, where)
	return nil`,
			[]int{0, 0, 0, 0, 4, 4},
		},
		{
			"two locals with one reader each group the same way every time",
			`left, right := read()
	use(left)
	use(right)
	return nil`,
			[]int{0, 0, 0, 3, 3},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte("package app\n\nfunc Index() error {\n\t" + testCase.body + "\n}\n")

			decls, err := syntax.Declarations(context.Background(), "app/app.go", source)
			if err != nil {
				t.Fatalf("Declarations() error = %v", err)
			}

			var fn entities.Decl

			for _, decl := range decls {
				if decl.Key.Name == "Index" {
					fn = decl
				}
			}

			for run := range runs {
				sliced, ok, err := Slice(context.Background(), fn, source)
				if err != nil || !ok {
					t.Fatalf("Slice() ok = %v, error = %v", ok, err)
				}

				if got := atomShape(sliced); !slices.Equal(got, testCase.want) {
					t.Fatalf("run %d: atoms = %v, want %v", run, got, testCase.want)
				}
			}
		})
	}
}
