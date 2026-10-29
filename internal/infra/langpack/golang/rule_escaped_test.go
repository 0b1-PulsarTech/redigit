package golang

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

const escapedTypes = `package fixture

type Missing struct{}

func (Missing) Error() string { return "missing" }

func (Missing) Unwrap() error { return nil }

func (Missing) StatusCode() int { return 404 }

func (Missing) Helper() {}

type statusCoder interface{ StatusCode() int }
`

func TestEscapedRule(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		use    string
		wantBy entities.DeclKey
		want   []string
		reject string
	}{
		{
			"a value wrapped by %w into an error",
			"import \"fmt\"\n\nfunc Render() error { return fmt.Errorf(\"x: %w\", Missing{}) }\n",
			testkit.DeclKey("use.go", entities.KindFunc, "Render"),
			[]string{"Missing.Error", "Missing.Unwrap"},
			"Missing.Helper",
		},
		{
			"a value held as any and asserted to a duck-typed port",
			"func Code(v any) int { return v.(statusCoder).StatusCode() }\n\n" +
				"func Use() int { return Code(Missing{}) }\n",
			testkit.DeclKey("use.go", entities.KindFunc, "Use"),
			[]string{"Missing.StatusCode"},
			"Missing.Helper",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, map[string]string{
				"types.go": escapedTypes,
				"use.go":   "package fixture\n\n" + testCase.use,
			})

			edges, err := escapedRule{source: typed.NewSource(nil)}.Edges(
				context.Background(), snap, decls,
			)
			if err != nil {
				t.Fatalf("Edges() error = %v", err)
			}

			got := testkit.EdgeSet(edges)

			for _, method := range testCase.want {
				want := entities.Edge{
					Dependent: testCase.wantBy,
					Provider:  testkit.DeclKey("types.go", entities.KindMethod, method),
					Reason:    entities.ReasonImplements,
				}
				if !got[want] {
					t.Fatalf("Edges() = %+v, missing %+v", edges, want)
				}
			}

			for edge := range got {
				if edge.Provider.Name == testCase.reject {
					t.Fatalf("Edges() waits for a method nothing reaches by name: %+v", edge)
				}
			}
		})
	}
}

func TestEscapedRuleSkipsTheTypesOwnMethods(t *testing.T) {
	t.Parallel()

	snap, decls := testkit.Fixture(t, map[string]string{
		"types.go": escapedTypes + `
func (m Missing) Wrap() error { return error(m) }
`,
	})

	edges, err := escapedRule{source: typed.NewSource(nil)}.Edges(
		context.Background(), snap, decls,
	)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("Edges() = %+v, want none: siblings of one type do not wait for each other", edges)
	}
}
