package wiring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func boundTree(t *testing.T, file string, decls ...string) (entities.Snapshot, []entities.Span) {
	t.Helper()

	root := t.TempDir()
	body := "package aiturn\n\n"

	spans := make([]entities.Span, 0, len(decls))

	for _, decl := range decls {
		start := len(body)
		body += decl + "\n\n"
		spans = append(spans, entities.Span{Start: start, End: len(body) - 2})
	}

	if err := os.WriteFile(filepath.Join(root, file), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return entities.Snapshot{Root: root, Files: []string{file}}, spans
}

func boundDecl(file string, kind entities.DeclKind, name string, span entities.Span) entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: kind, Name: name},
		Unit: "aiturn",
		Span: span,
	}
}

func TestBoundRuleTiesAConstructorThatNamesABoundType(t *testing.T) {
	t.Parallel()

	const file = "reply.go"

	snap, spans := boundTree(t, file,
		"type replyExecutor struct{}",
		"func newReplyExecutor(deps ReplierDeps) replyExecutor {\n\treturn replyExecutor{}\n}",
		"func (replyExecutor) Reply() error { return nil }",
		"var _ Replier = replyExecutor{}",
	)

	replier := entities.Decl{
		Key:         entities.DeclKey{File: "replier.go", Kind: entities.KindType, Name: "Replier"},
		Unit:        "aiturn",
		IsInterface: true,
	}
	executor := boundDecl(file, entities.KindType, "replyExecutor", spans[0])
	constructor := boundDecl(file, entities.KindFunc, "newReplyExecutor", spans[1])
	reply := boundDecl(file, entities.KindMethod, "replyExecutor.Reply", spans[2])
	assertion := boundDecl(file, entities.KindValue, "_ Replier = replyExecutor{}", spans[3])

	edges, err := (Bound{}).Edges(
		t.Context(), snap, []entities.Decl{replier, executor, constructor, reply, assertion},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	waits := map[entities.DeclKey]bool{}

	for _, edge := range edges {
		if edge.Provider == reply.Key {
			waits[edge.Dependent] = true
		}
	}

	for _, want := range []entities.Decl{constructor, assertion} {
		if !waits[want.Key] {
			t.Fatalf("%s names a bound type and does not wait for its method", want.Key.Name)
		}
	}

	// Neither the method nor the type itself waits: they ARE the wait, and
	// "type T struct{}" compiles with no method at all.
	for _, never := range []entities.Decl{reply, executor} {
		if waits[never.Key] {
			t.Fatalf("%s waits for a method of the type it declares", never.Key.Name)
		}
	}
}

func TestBoundRuleIgnoresATypeNoInterfaceTakes(t *testing.T) {
	t.Parallel()

	const file = "plain.go"

	snap, spans := boundTree(t, file,
		"type plain struct{}",
		"func newPlain() plain { return plain{} }",
		"func (plain) Do() error { return nil }",
	)

	iface := entities.Decl{
		Key:         entities.DeclKey{File: "i.go", Kind: entities.KindType, Name: "Replier"},
		Unit:        "aiturn",
		IsInterface: true,
	}

	edges, err := (Bound{}).Edges(t.Context(), snap, []entities.Decl{
		iface,
		boundDecl(file, entities.KindType, "plain", spans[0]),
		boundDecl(file, entities.KindFunc, "newPlain", spans[1]),
		boundDecl(file, entities.KindMethod, "plain.Do", spans[2]),
	})
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edge(s) for a type no interface takes: %v", len(edges), edges)
	}
}

func TestMentionedTypesReadsCodeNotProse(t *testing.T) {
	t.Parallel()

	const file = "doc.go"

	snap, spans := boundTree(
		t,
		file,
		"// replyExecutor is described here but not named.\nfunc unrelated() string {\n\treturn \"replyExecutor\"\n}",
	)

	got := mentionedTypes(
		boundDecl(file, entities.KindFunc, "unrelated", spans[0]),
		snap,
		map[string][]byte{},
	)

	for _, name := range got {
		if name == "replyExecutor" {
			t.Fatalf("a name in a comment and a string counted as a mention: %v", got)
		}
	}
}

func TestBoundRuleDoesNotMakeAMethodWaitForItsSiblings(t *testing.T) {
	t.Parallel()

	const file = "reply.go"

	snap, spans := boundTree(t, file,
		"type replyExecutor struct{}",
		"func (replyExecutor) Reply() error { return nil }",
		"func (replyExecutor) Close() error { return nil }",
		"var _ Replier = replyExecutor{}",
	)

	replier := entities.Decl{
		Key:         entities.DeclKey{File: "i.go", Kind: entities.KindType, Name: "Replier"},
		Unit:        "aiturn",
		IsInterface: true,
	}
	reply := boundDecl(file, entities.KindMethod, "replyExecutor.Reply", spans[1])
	closeM := boundDecl(file, entities.KindMethod, "replyExecutor.Close", spans[2])

	edges, err := (Bound{}).Edges(t.Context(), snap, []entities.Decl{
		replier,
		boundDecl(file, entities.KindType, "replyExecutor", spans[0]),
		reply,
		closeM,
		boundDecl(file, entities.KindValue, "_ Replier = replyExecutor{}", spans[3]),
	})
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	for _, edge := range edges {
		if edge.Dependent == reply.Key || edge.Dependent == closeM.Key {
			t.Fatalf(
				"%s waits for %s: the methods ARE what the wait is for",
				edge.Dependent.Name, edge.Provider.Name,
			)
		}
	}
}

func TestBoundRuleDoesNotMakeATypeWaitForItsOwnMethods(t *testing.T) {
	t.Parallel()

	const file = "reply.go"

	snap, spans := boundTree(t, file,
		"type replyExecutor struct{}",
		"func (replyExecutor) Reply() error { return nil }",
		"var _ Replier = replyExecutor{}",
	)

	executor := boundDecl(file, entities.KindType, "replyExecutor", spans[0])

	edges, err := (Bound{}).Edges(t.Context(), snap, []entities.Decl{
		{
			Key:         entities.DeclKey{File: "i.go", Kind: entities.KindType, Name: "Replier"},
			Unit:        "aiturn",
			IsInterface: true,
		},
		executor,
		boundDecl(file, entities.KindMethod, "replyExecutor.Reply", spans[1]),
		boundDecl(file, entities.KindValue, "_ Replier = replyExecutor{}", spans[2]),
	})
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	for _, edge := range edges {
		if edge.Dependent == executor.Key {
			t.Fatalf("the type waits for %s, one of its own methods", edge.Provider.Name)
		}
	}
}

func TestBoundRuleBindsATypeHandedToAStructField(t *testing.T) {
	t.Parallel()

	const file = "routing_test.go"

	snap, spans := boundTree(t, file,
		"type noopHandover struct{}",
		"func (noopHandover) HandToHuman() error { return nil }",
		"func newDeps() Deps {\n\treturn Deps{Handover: noopHandover{}}\n}",
	)

	iface := entities.Decl{
		Key:         entities.DeclKey{File: "i.go", Kind: entities.KindType, Name: "Replier"},
		Unit:        "aiturn",
		IsInterface: true,
	}
	hand := boundDecl(file, entities.KindMethod, "noopHandover.HandToHuman", spans[1])
	builder := boundDecl(file, entities.KindFunc, "newDeps", spans[2])

	edges, err := (Bound{}).Edges(t.Context(), snap, []entities.Decl{
		iface, boundDecl(file, entities.KindType, "noopHandover", spans[0]), hand, builder,
	})
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	for _, edge := range edges {
		if edge.Dependent == builder.Key && edge.Provider == hand.Key {
			return
		}
	}

	t.Fatalf("the builder does not wait for the method of the type it hands over; got %v", edges)
}

func TestBoundRuleDoesNotBindAPlainConstruction(t *testing.T) {
	t.Parallel()

	const file = "plain.go"

	snap, spans := boundTree(t, file,
		"type plain struct{}",
		"func (plain) Do() error { return nil }",
		"func use() error {\n\tp := plain{}\n\n\treturn p.Do()\n}",
	)

	edges, err := (Bound{}).Edges(t.Context(), snap, []entities.Decl{
		{
			Key:         entities.DeclKey{File: "i.go", Kind: entities.KindType, Name: "Replier"},
			Unit:        "aiturn",
			IsInterface: true,
		},
		boundDecl(file, entities.KindType, "plain", spans[0]),
		boundDecl(file, entities.KindMethod, "plain.Do", spans[1]),
		boundDecl(file, entities.KindFunc, "use", spans[2]),
	})
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if len(edges) != 0 {
		t.Fatalf("got %d edge(s) for a value built and kept: %v", len(edges), edges)
	}
}
