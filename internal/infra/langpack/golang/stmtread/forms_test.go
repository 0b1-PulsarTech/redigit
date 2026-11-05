package stmtread

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
)

type snippetCase struct {
	name    string
	snippet string
	want    linting.Statement
}

func readSnippets(tb testing.TB, source string, snippets []string) []linting.Statement {
	tb.Helper()

	spans := map[entities.DeclKey]entities.Span{}
	keys := make([]entities.DeclKey, len(snippets))

	for index, snippet := range snippets {
		start := strings.Index(source, snippet)
		if start < 0 {
			tb.Fatalf("snippet %q is not in the source", snippet)
		}

		keys[index] = entities.DeclKey{
			File: "a.go", Kind: entities.KindStmt, Name: "F#" + strconv.Itoa(index),
		}
		spans[keys[index]] = entities.Span{Start: start, End: start + len(snippet)}
	}

	syntax := Reader{read: func(context.Context, string) ([]byte, error) {
		return []byte(source), nil
	}}

	got, err := syntax.Statements(context.Background(), "a.go", spans)
	if err != nil {
		tb.Fatalf("Statements() err = %v", err)
	}

	facts := make([]linting.Statement, len(snippets))
	for index, key := range keys {
		facts[index] = got[key]
	}

	return facts
}

func runSnippetCases(t *testing.T, testCases []snippetCase) {
	t.Helper()

	const source = "package a\n\nfunc F() error {\n" +
		"\terr = g()\n" +
		"\tif err != nil {\n\t\treturn err\n\t}\n" +
		"\tx, err := h()\n" +
		"\terr := k()\n" +
		"\tother = g()\n" +
		"\tif x != nil {\n\t}\n" +
		"\tif err := g(); err != nil {\n\t}\n" +
		"\tif ok && err != nil {\n\t}\n" +
		"\tif (err != nil) {\n\t}\n" +
		"\tif err == nil {\n\t}\n" +
		"\t_, err = h()\n" +
		"\t// a note before the pair\n" +
		"\ty, err := h()\n" +
		"\t// a note before the check\n" +
		"\tif err != nil {\n\t}\n" +
		"\tvar err error\n\tvar a, err = g()\n\tvar other error\n\tconst err = 1\n" +
		"\treturn nil\n}\n"

	snippets := make([]string, len(testCases))
	for index, testCase := range testCases {
		snippets[index] = testCase.snippet
	}

	facts := readSnippets(t, source, snippets)

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if facts[index] != testCase.want {
				t.Fatalf("Statements()[%d] = %+v, want %+v", index, facts[index], testCase.want)
			}
		})
	}
}

func TestReadStatementReadsTheFormOfEachAssignment(t *testing.T) {
	t.Parallel()

	runSnippetCases(t, []snippetCase{
		{
			name: "err = is a plain assignment", snippet: "err = g()",
			want: linting.Statement{Assign: linting.FormAssign},
		},
		{
			name:    "x, err := is a define, whether it binds err is told by the domain",
			snippet: "x, err := h()",
			want:    linting.Statement{Assign: linting.FormDefine},
		},
		{
			name: "err := is a define", snippet: "err := k()",
			want: linting.Statement{Assign: linting.FormDefine},
		},
		{
			name: "_, err = is a plain assignment", snippet: "_, err = h()",
			want: linting.Statement{Assign: linting.FormAssign},
		},
		{name: "another name is no error variable", snippet: "other = g()"},
		{name: "a return is neither", snippet: "return nil"},
		{
			name:    "a comment before the assignment does not hide it",
			snippet: "// a note before the pair\n\ty, err := h()",
			want:    linting.Statement{Assign: linting.FormDefine},
		},
		{
			name: "var err T binds err and starts no pair", snippet: "var err error",
			want: linting.Statement{Binds: true},
		},
		{
			name: "var a, err = g() starts a pair", snippet: "var a, err = g()",
			want: linting.Statement{Assign: linting.FormDefine},
		},
		{name: "a var of another name is no error variable", snippet: "var other error"},
		{name: "a const is not an error variable", snippet: "const err = 1"},
	})
}

func TestReadStatementReadsWhichStatementsCheckErr(t *testing.T) {
	t.Parallel()

	runSnippetCases(t, []snippetCase{
		{
			name: "if err != nil is a check", snippet: "if err != nil {\n\t\treturn err\n\t}",
			want: linting.Statement{Checks: true},
		},
		{name: "a test of another variable checks nothing", snippet: "if x != nil {\n\t}"},
		{
			name:    "an if with its own err in the init is one statement",
			snippet: "if err := g(); err != nil {\n\t}",
		},
		{
			name: "an err test inside && still checks", snippet: "if ok && err != nil {\n\t}",
			want: linting.Statement{Checks: true},
		},
		{
			name: "a parenthesised test still checks", snippet: "if (err != nil) {\n\t}",
			want: linting.Statement{Checks: true},
		},
		{name: "err == nil is not the check", snippet: "if err == nil {\n\t}"},
		{
			name:    "a comment before the check does not hide it",
			snippet: "// a note before the check\n\tif err != nil {\n\t}",
			want:    linting.Statement{Checks: true},
		},
	})
}
