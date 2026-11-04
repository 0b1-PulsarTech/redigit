package router

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

func TestHeads(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		site entities.DeclKey
		want []entities.DeclKey
	}{
		{
			name: "a func is its own head",
			site: fnKey("Setup"),
			want: []entities.DeclKey{fnKey("Setup")},
		},
		{
			name: "a statement has its func and its method as heads",
			site: stmtKey("Setup", "3"),
			want: []entities.DeclKey{
				stmtKey("Setup", "3"),
				fnKey("Setup"),
				{File: "api/api.go", Kind: entities.KindMethod, Name: "Setup"},
			},
		},
		{
			name: "only the last hash cuts the name",
			site: stmtKey("Odd#Name", "3"),
			want: []entities.DeclKey{
				stmtKey("Odd#Name", "3"),
				fnKey("Odd#Name"),
				{File: "api/api.go", Kind: entities.KindMethod, Name: "Odd#Name"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := heads(testCase.site); !slices.Equal(got, testCase.want) {
				t.Fatalf("heads() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestStitchedCalls(t *testing.T) {
	t.Parallel()

	var (
		head   = fnKey("Mount")
		first  = facts.Call{Callee: "a"}
		second = facts.Call{Callee: "b"}
		third  = facts.Call{Callee: "c"}
	)

	testCases := []struct {
		name  string
		decls []entities.Decl
		calls map[entities.DeclKey][]facts.Call
		site  entities.DeclKey
		want  []facts.Call
	}{
		{
			name:  "no statement leaves the facts as they are",
			decls: []entities.Decl{declAt(head, 0)},
			calls: map[entities.DeclKey][]facts.Call{head: {first}},
			site:  head,
			want:  []facts.Call{first},
		},
		{
			name: "the calls of the statements join the head",
			decls: []entities.Decl{
				declAt(stmtKey("Mount", "0"), 0),
				declAt(stmtKey("Mount", "1"), 5),
			},
			calls: map[entities.DeclKey][]facts.Call{
				head: {first}, stmtKey("Mount", "0"): {second}, stmtKey("Mount", "1"): {third},
			},
			site: head,
			want: []facts.Call{first, second, third},
		},
		{
			name:  "the calls of the statements of a method join the method",
			decls: []entities.Decl{declAt(stmtKey("Mount", "0"), 0)},
			calls: map[entities.DeclKey][]facts.Call{
				{File: "api/api.go", Kind: entities.KindMethod, Name: "Mount"}: {first},
				stmtKey("Mount", "0"): {second},
			},
			site: entities.DeclKey{File: "api/api.go", Kind: entities.KindMethod, Name: "Mount"},
			want: []facts.Call{first, second},
		},
		{
			name:  "a statement answers for itself only",
			decls: []entities.Decl{declAt(stmtKey("Mount", "0"), 0)},
			calls: map[entities.DeclKey][]facts.Call{
				head:                  {first},
				stmtKey("Mount", "0"): {second},
			},
			site: stmtKey("Mount", "0"),
			want: []facts.Call{second},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			known := newStitched(world{calls: testCase.calls}, testCase.decls)
			if got := known.Calls(testCase.site); !slices.EqualFunc(got, testCase.want,
				func(left, right facts.Call) bool { return left.Callee == right.Callee }) {
				t.Fatalf("Calls() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestEnclosing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		site entities.DeclKey
		want entities.DeclKey
	}{
		{name: "a func is its own owner", site: fnKey("Setup"), want: fnKey("Setup")},
		{
			name: "a statement belongs to the func it was cut from",
			site: stmtKey("Setup", "3"),
			want: fnKey("Setup"),
		},
		{
			name: "a method is named by its func key",
			site: entities.DeclKey{File: "api/api.go", Kind: entities.KindMethod, Name: "Setup"},
			want: fnKey("Setup"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := enclosing(testCase.site); got != testCase.want {
				t.Fatalf("enclosing(%s) = %s, want %s", testCase.site, got, testCase.want)
			}
		})
	}
}
