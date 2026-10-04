package golang

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const conversionTypes = `package fixture

type Reader interface{ Read() string }

type Repo struct{}

func (Repo) Read() string { return "" }

func (Repo) Write(string) {}
`

func TestTypedConversionRule(t *testing.T) {
	t.Parallel()

	read := declKey("types.go", entities.KindMethod, "Repo.Read")

	testCases := []struct {
		name   string
		use    string
		wantBy entities.DeclKey
	}{
		{
			"an argument handed to an interface parameter",
			"func Take(Reader) {}\n\nfunc Wire() { Take(Repo{}) }\n",
			declKey("use.go", entities.KindFunc, "Wire"),
		},
		{
			"a return of a concrete value from an interface result",
			"func New() Reader { return &Repo{} }\n",
			declKey("use.go", entities.KindFunc, "New"),
		},
		{
			"a blank assertion",
			"var _ Reader = (*Repo)(nil)\n",
			declKey("use.go", entities.KindValue, "_ Reader = (*Repo)(nil)"),
		},
		{
			"a field of a composite literal",
			"type Holder struct{ R Reader }\n\nvar Held = Holder{R: Repo{}}\n",
			declKey("use.go", entities.KindValue, "Held"),
		},
		{
			"an assignment to an interface variable",
			"func Set() { var r Reader; r = Repo{}; _ = r }\n",
			declKey("use.go", entities.KindFunc, "Set"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := typedFixture(t, map[string]string{
				"types.go": conversionTypes,
				"use.go":   "package fixture\n\n" + testCase.use,
			})

			edges, err := typedConversionRule{source: newTypedSource(nil)}.Edges(
				context.Background(), snap, decls,
			)
			if err != nil {
				t.Fatalf("Edges() error = %v", err)
			}

			want := entities.Edge{
				Dependent: testCase.wantBy,
				Provider:  read,
				Reason:    entities.ReasonImplements,
			}

			got := edgeSet(edges)
			if !got[want] {
				t.Fatalf("Edges() = %+v, missing %+v", edges, want)
			}

			// Only what the interface lists is needed: Write is not.
			for edge := range got {
				if edge.Provider.Name == "Repo.Write" {
					t.Fatalf("Edges() waits for a method the interface does not list: %+v", edge)
				}
			}
		})
	}
}

func TestTypedConversionRuleIgnoresInterfaceToInterface(t *testing.T) {
	t.Parallel()

	snap, decls := typedFixture(t, map[string]string{
		"types.go": conversionTypes,
		"use.go": `package fixture

type Wide interface {
	Reader
	Write(string)
}

func Narrow(w Wide) Reader { return w }
`,
	})

	edges, err := typedConversionRule{source: newTypedSource(nil)}.Edges(
		context.Background(), snap, decls,
	)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("Edges() = %+v, want none: an interface owns no method to wait for", edges)
	}
}

func TestTypedSourceWithoutModuleLoadsNothing(t *testing.T) {
	t.Parallel()

	facts, err := newTypedSource(nil).Facts(
		context.Background(), entities.Snapshot{Root: t.TempDir()}, nil,
	)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	if len(facts.Forced)+len(facts.Converted) != 0 {
		t.Fatalf("Facts() = %+v, want none", facts)
	}
}
