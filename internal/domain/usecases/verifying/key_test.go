package verifying

import (
	"regexp"
	"testing"
)

func TestKey(t *testing.T) {
	t.Parallel()

	base := Unit{
		Module: Module{Dir: "m", Path: "ex/m", Language: "go", Subtree: "t1"},
		Deps:   []Module{{Dir: "d", Path: "ex/d", Subtree: "t2"}},
	}
	baseKey := Key(base, "go1;build;vet;tags=a")

	mutate := func(edit func(unit *Unit)) Unit {
		unit := base
		unit.Deps = append([]Module(nil), base.Deps...)
		edit(&unit)

		return unit
	}

	testCases := []struct {
		name     string
		unit     Unit
		identity string
		wantSame bool
	}{
		{"identical input", base, "go1;build;vet;tags=a", true},
		{"identity changes (tags, version, argv, go.work)", base, "go1;build;vet;tags=b", false},
		{
			"module subtree",
			mutate(func(unit *Unit) { unit.Subtree = "t9" }),
			"go1;build;vet;tags=a",
			false,
		},
		{
			"module path",
			mutate(func(unit *Unit) { unit.Path = "ex/other" }),
			"go1;build;vet;tags=a",
			false,
		},
		{"module dir", mutate(func(unit *Unit) { unit.Dir = "n" }), "go1;build;vet;tags=a", false},
		{
			"language",
			mutate(func(unit *Unit) { unit.Language = "dart" }),
			"go1;build;vet;tags=a",
			false,
		},
		{
			"dependency subtree",
			mutate(func(unit *Unit) { unit.Deps[0].Subtree = "t9" }),
			"go1;build;vet;tags=a",
			false,
		},
		{
			"dependency added",
			mutate(func(unit *Unit) { unit.Deps = nil }),
			"go1;build;vet;tags=a",
			false,
		},
		{
			"field boundaries are not ambiguous",
			mutate(
				func(unit *Unit) { unit.Dir, unit.Path = "mex", "/m" },
			),
			"go1;build;vet;tags=a",
			false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Key(testCase.unit, testCase.identity)
			if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(got) {
				t.Fatalf("key = %q, want 64 hex digits", got)
			}

			if (got == baseKey) != testCase.wantSame {
				t.Fatalf("same = %v, want %v", got == baseKey, testCase.wantSame)
			}
		})
	}
}

func TestKeyFollowsTheWorkspace(t *testing.T) {
	t.Parallel()

	module := Module{Dir: "m", Path: "ex/m", Language: "go", Subtree: "t1"}
	plain := Key(Unit{Module: module}, "id")

	module.Workspace = "use ./m"
	shipped := Key(Unit{Module: module}, "id")

	module.Workspace = "use ./n"
	changed := Key(Unit{Module: module}, "id")

	if plain == shipped || shipped == changed {
		t.Fatalf("keys = %q %q %q, want one per workspace text", plain, shipped, changed)
	}
}
