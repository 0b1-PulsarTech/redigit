package wiring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var receiverOfAssertionCases = []struct {
	name string
	text string
	want string
}{
	{
		name: "a pointer assertion",
		text: "_ Codec = (*Registry)(nil)",
		want: "Registry",
	},
	{
		name: "the span's own var keyword",
		text: "var _ Codec = (*Registry)(nil)",
		want: "Registry",
	},
	{
		// A span carries the doc comment too, and prepending "var"
		// unconditionally would put it before the comment.
		name: "a documented assertion",
		text: "// Registry satisfies Codec.\nvar _ Codec = (*Registry)(nil)",
		want: "Registry",
	},
	{
		// The form a pattern misses, and the one that failed 224 trees.
		name: "a generic interface",
		text: "_ ruletypes.Rule[string] = (*factEqualsRule)(nil)",
		want: "factEqualsRule",
	},
	{
		name: "a generic concrete type",
		text: "_ webwrap.Context = (*requestContext[any, any])(nil)",
		want: "requestContext",
	},
	{
		name: "a value assertion with no pointer",
		text: "_ flowuc.ActionCatalog = unionActionCatalog(nil)",
		want: "unionActionCatalog",
	},
	{
		// The form that cost 300 of 327 failing trees: a VALUE composite
		// literal is not a conversion, so a check written for conversions
		// never saw it.
		name: "a value composite literal",
		text: "_ Replier = replyExecutor{}",
		want: "replyExecutor",
	},
	{
		name: "a pointer to a composite literal",
		text: "_ ReplyEnricher = &BotReplyEnricher{}",
		want: "BotReplyEnricher",
	},
	{
		name: "a qualified interface with a value literal",
		text: "_ aiturn.ReplyEnricher = BotReplyEnricher{}",
		want: "BotReplyEnricher",
	},
	{
		name: "a generic type with a value literal",
		text: "_ ruletypes.Rule[string] = factEqualsRule{}",
		want: "factEqualsRule",
	},
	{
		name: "a qualified interface and a local type",
		text: "_ http.Handler = (*Server)(nil)",
		want: "Server",
	},
	{
		// A type from another package has no methods in this unit, and
		// its qualifier hides two levels down: testing only the outermost
		// expression let it through as "Thing".
		name: "a type from another package",
		text: "_ Iface = (*other.Thing)(nil)",
		want: "",
	},
	{
		name: "a value literal from another package",
		text: "_ Iface = other.Thing{}",
		want: "",
	},
	{
		name: "not an assertion at all",
		text: "_ = compute()",
		want: "",
	},
}

func TestReceiverOfAssertion(t *testing.T) {
	t.Parallel()

	for _, testCase := range receiverOfAssertionCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, found := receiverOfAssertion(testCase.text)
			if testCase.want == "" {
				if found {
					t.Fatalf("receiverOfAssertion(%q) = %q, want none", testCase.text, got)
				}

				return
			}

			if !found || got != testCase.want {
				t.Fatalf("receiverOfAssertion(%q) = %q, want %q", testCase.text, got, testCase.want)
			}
		})
	}
}

func assertionTree(t *testing.T, body string) (entities.Snapshot, entities.Span) {
	t.Helper()

	root := t.TempDir()
	source := "package permitek\n\n" + body

	if err := os.WriteFile(filepath.Join(root, "codec.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	start := len("package permitek\n\n")

	return entities.Snapshot{Root: root, Files: []string{"codec.go"}},
		entities.Span{Start: start, End: len(source)}
}

func TestAssertionRuleTiesTheAssertionToTheReceiversMethods(t *testing.T) {
	t.Parallel()

	snap, span := assertionTree(t, "var _ Codec = (*Registry)(nil)\n")

	assertion := entities.Decl{
		Key: entities.DeclKey{
			File: "codec.go",
			Kind: entities.KindValue,
			Name: "_ Codec = (*Registry)(nil)",
		},
		Unit: "permitek",
		Span: span,
	}
	encode := entities.Decl{
		Key: entities.DeclKey{
			File: "codec.go",
			Kind: entities.KindMethod,
			Name: "Registry.Encode",
		},
		Unit: "permitek",
	}
	decode := entities.Decl{
		Key: entities.DeclKey{
			File: "other.go",
			Kind: entities.KindMethod,
			Name: "Registry.Decode",
		},
		Unit: "permitek",
	}
	elsewhere := entities.Decl{
		Key:  entities.DeclKey{File: "far.go", Kind: entities.KindMethod, Name: "Registry.Encode"},
		Unit: "somewhere/else",
	}

	edges, err := (Assertion{}).Edges(
		t.Context(), snap, []entities.Decl{assertion, encode, decode, elsewhere},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	got := map[entities.DeclKey]bool{}

	for _, edge := range edges {
		if edge.Dependent != assertion.Key {
			t.Fatalf("an edge hangs off %s, not the assertion", edge.Dependent.String())
		}

		got[edge.Provider] = true
	}

	for _, want := range []entities.Decl{encode, decode} {
		if !got[want.Key] {
			t.Fatalf("the assertion does not wait for %s", want.Key.String())
		}
	}

	// A same-named receiver in another package is another type entirely.
	if got[elsewhere.Key] {
		t.Fatal("the assertion waits for a method of a same-named type in another unit")
	}
}

func TestAssertionRuleIgnoresAnOrdinaryValue(t *testing.T) {
	t.Parallel()

	snap, span := assertionTree(t, "var limit = 10\n")

	value := entities.Decl{
		Key:  entities.DeclKey{File: "codec.go", Kind: entities.KindValue, Name: "limit"},
		Unit: "permitek",
		Span: span,
	}
	method := entities.Decl{
		Key: entities.DeclKey{
			File: "codec.go",
			Kind: entities.KindMethod,
			Name: "Registry.Encode",
		},
		Unit: "permitek",
	}

	edges, err := (Assertion{}).Edges(t.Context(), snap, []entities.Decl{value, method})
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edge(s) from a plain value: %v", len(edges), edges)
	}
}
