package golang

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestReturnedConcreteTypes(t *testing.T) {
	t.Parallel()

	interfaces := map[string]bool{"Rule": true, "Handler": true}

	testCases := []struct {
		name string
		text string
		want []string
	}{
		{
			// The form that failed 416 trees: a generic interface from
			// another package, returning a pointer to a local type.
			name: "a generic qualified interface",
			text: "func NewEquals() ruletypes.Rule[string] {\n\treturn &factEqualsRule{}\n}",
			want: []string{"factEqualsRule"},
		},
		{
			// A declaration's span carries its doc comment, and real code
			// comments almost every exported constructor. Requiring the text
			// to start with "func" skipped all of them.
			name: "a documented constructor",
			text: "// NewEquals builds a Rule.\n" +
				"func NewEquals() ruletypes.Rule[string] {\n\treturn &factEqualsRule{}\n}",
			want: []string{"factEqualsRule"},
		},
		{
			name: "a value, not a pointer",
			text: "func New() Handler {\n\treturn handler{}\n}",
			want: []string{"handler"},
		},
		{
			name: "two returns of two types",
			text: "func New(on bool) Rule {\n\tif on {\n\t\treturn &a{}\n\t}\n\n\treturn &b{}\n}",
			want: []string{"a", "b"},
		},
		{
			// The gate: returning the concrete type as itself converts
			// nothing and needs no method.
			name: "the result type is the concrete type",
			text: "func New() *thing {\n\treturn &thing{}\n}",
			want: nil,
		},
		{
			name: "the result is an interface but nothing is constructed",
			text: "func New(r Rule) Rule {\n\treturn r\n}",
			want: nil,
		},
		{
			name: "a type from another package",
			text: "func New() Rule {\n\treturn &other.Thing{}\n}",
			want: nil,
		},
		{
			name: "not a function at all",
			text: "var x = 1",
			want: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := ReturnedConcreteTypes(testCase.text, interfaces)

			if len(got) != len(testCase.want) {
				t.Fatalf("returnedConcreteTypes() = %v, want %v", got, testCase.want)
			}

			for index, name := range testCase.want {
				if got[index] != name {
					t.Fatalf("returnedConcreteTypes() = %v, want %v", got, testCase.want)
				}
			}
		})
	}
}

func TestConversionRuleTiesAConstructorToTheMethods(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := "package commonrules\n\n" +
		"func NewEquals() ruletypes.Rule[string] {\n\treturn &factEqualsRule{}\n}\n"

	if err := os.WriteFile(
		filepath.Join(root, "fact_equals.go"),
		[]byte(source),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	start := len("package commonrules\n\n")
	snap := entities.Snapshot{Root: root, Files: []string{"fact_equals.go"}}

	constructor := entities.Decl{
		Key:  entities.DeclKey{File: "fact_equals.go", Kind: entities.KindFunc, Name: "NewEquals"},
		Unit: "commonrules",
		Span: entities.Span{Start: start, End: len(source)},
	}
	evaluate := entities.Decl{
		Key: entities.DeclKey{
			File: "fact_equals.go",
			Kind: entities.KindMethod,
			Name: "factEqualsRule.Evaluate",
		},
		Unit: "commonrules",
	}
	rule := entities.Decl{
		Key: entities.DeclKey{
			File: "ruletypes/rule.go",
			Kind: entities.KindType,
			Name: "Rule",
		},
		Unit:        "ruletypes",
		IsInterface: true,
	}

	edges, err := (conversionRule{}).Edges(
		t.Context(), snap, []entities.Decl{constructor, evaluate, rule},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	found := false

	for _, edge := range edges {
		if edge.Dependent == constructor.Key && edge.Provider == evaluate.Key {
			found = true
		}
	}

	if !found {
		t.Fatalf(
			"the constructor does not wait for the method that makes its return type "+
				"satisfy the interface; got %d edge(s)", len(edges),
		)
	}
}

// With no interface in the graph the rule has nothing to gate on, and must
// not wait on every construction in the repository.
func TestConversionRuleIsSilentWithNoInterface(t *testing.T) {
	t.Parallel()

	edges, err := (conversionRule{}).Edges(
		t.Context(),
		entities.Snapshot{},
		[]entities.Decl{{
			Key:  entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "New"},
			Unit: "a",
		}},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edge(s) with no interface known", len(edges))
	}
}
