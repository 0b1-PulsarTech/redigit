package golang

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestShowingTags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		constraint string
		tags       []string
		want       []string
		wantOK     bool
	}{
		{
			"a negated tag is shown by dropping it",
			"//go:build !dev",
			[]string{"dev", "x"},
			[]string{"x"},
			true,
		},
		{
			"a missing tag is shown by adding it",
			"//go:build integration",
			nil,
			[]string{"integration"},
			true,
		},
		{"a conjunction needs both", "//go:build a && b", []string{"a"}, []string{"a", "b"}, true},
		{"a contradiction is shown by nothing", "//go:build a && !a", nil, nil, false},
		{"not a constraint at all", "// just a comment", nil, nil, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, ok := showingTags(testCase.constraint, testCase.tags)
			if ok != testCase.wantOK || (ok && !reflect.DeepEqual(got, testCase.want)) {
				t.Fatalf(
					"showingTags() = %v, %v; want %v, %v",
					got,
					ok,
					testCase.want,
					testCase.wantOK,
				)
			}
		})
	}
}

func TestTypedSourceChecksTheFilesTheTagsHide(t *testing.T) {
	t.Parallel()

	snap, decls := typedFixture(t, map[string]string{
		"verifier.go": "package fixture\n\nfunc NewGateway() int { return 1 }\n",
		"verifier_prod.go": `//go:build !dev

package fixture

func NewVerifier() int { return NewGateway() }
`,
		"verifier_dev.go": "//go:build dev\n\npackage fixture\n\nfunc NewVerifier() int { return 0 }\n",
	})

	facts, err := newTypedSource([]string{"dev"}).Facts(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	want := use{
		Dependent: declKey("verifier_prod.go", entities.KindFunc, "NewVerifier"),
		Provider:  declKey("verifier.go", entities.KindFunc, "NewGateway"),
	}

	if slices.Contains(facts.Uses, want) {
		return
	}

	t.Fatalf(
		"Uses = %+v, missing %+v: the file hidden by the tags was never checked",
		facts.Uses,
		want,
	)
}
