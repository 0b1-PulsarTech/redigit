package verifying

import (
	"slices"
	"testing"
)

func depDirs(unit Unit) []string {
	dirs := make([]string, 0, len(unit.Deps))
	for _, dep := range unit.Deps {
		dirs = append(dirs, dep.Dir)
	}

	return dirs
}

func TestResolve(t *testing.T) {
	t.Parallel()

	alpha := Module{Dir: "alpha", Path: "ex/alpha", Subtree: "s1"}
	beta := Module{
		Dir:      "beta",
		Path:     "ex/beta",
		Subtree:  "s2",
		Requires: []string{"ex/alpha", "x/ext"},
	}
	gamma := Module{Dir: "gamma", Path: "ex/gamma", Subtree: "s3", Requires: []string{"ex/beta"}}
	loopA := Module{Dir: "la", Path: "ex/la", Requires: []string{"ex/lb"}}
	loopB := Module{Dir: "lb", Path: "ex/lb", Requires: []string{"ex/la"}}

	testCases := []struct {
		name     string
		modules  []Module
		wantDirs []string
		wantDeps map[string][]string
	}{
		{"none", nil, []string{}, nil},
		{
			"units come back in dir order with transitive deps",
			[]Module{gamma, alpha, beta},
			[]string{"alpha", "beta", "gamma"},
			map[string][]string{"alpha": {}, "beta": {"alpha"}, "gamma": {"alpha", "beta"}},
		},
		{
			"an external requirement is no dependency",
			[]Module{beta},
			[]string{"beta"},
			map[string][]string{"beta": {}},
		},
		{
			"modules under one workspace see each other, others stay apart",
			[]Module{
				{Dir: "alpha", Path: "ex/alpha", Workspace: "W"},
				{Dir: "beta", Path: "ex/beta", Workspace: "W"},
				{Dir: "gamma", Path: "ex/gamma", Workspace: "V"},
				{Dir: "solo", Path: "ex/solo"},
			},
			[]string{"alpha", "beta", "gamma", "solo"},
			map[string][]string{"alpha": {"beta"}, "beta": {"alpha"}, "gamma": {}, "solo": {}},
		},
		{
			"a workspace and a require edge name a dependency once",
			[]Module{
				{Dir: "alpha", Path: "ex/alpha", Workspace: "W", Requires: []string{"ex/beta"}},
				{Dir: "beta", Path: "ex/beta", Workspace: "W"},
			},
			[]string{"alpha", "beta"},
			map[string][]string{"alpha": {"beta"}, "beta": {"alpha"}},
		},
		{
			"a cycle terminates",
			[]Module{loopA, loopB},
			[]string{"la", "lb"},
			map[string][]string{"la": {"lb"}, "lb": {"la"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			units := Resolve(testCase.modules)
			dirs := make([]string, 0, len(units))

			for _, unit := range units {
				dirs = append(dirs, unit.Dir)

				if want := testCase.wantDeps[unit.Dir]; !slices.Equal(depDirs(unit), want) {
					t.Fatalf("deps of %s = %v, want %v", unit.Dir, depDirs(unit), want)
				}
			}

			if !slices.Equal(dirs, testCase.wantDirs) {
				t.Fatalf("dirs = %v, want %v", dirs, testCase.wantDirs)
			}
		})
	}
}

func TestResolveKeepsEveryModuleOfASharedPath(t *testing.T) {
	t.Parallel()

	user := Module{Dir: "user", Path: "ex/user", Requires: []string{"ex/dup"}}

	testCases := []struct {
		name    string
		modules []Module
		want    []string
	}{
		{
			"a require on a shared path depends on both modules",
			[]Module{
				{Dir: "one", Path: "ex/dup", Subtree: "s1"},
				{Dir: "two", Path: "ex/dup", Subtree: "s2"},
				user,
			},
			[]string{"one", "two"},
		},
		{
			"the order of the sharing modules does not matter",
			[]Module{
				{Dir: "two", Path: "ex/dup", Subtree: "s2"},
				{Dir: "one", Path: "ex/dup", Subtree: "s1"},
				user,
			},
			[]string{"one", "two"},
		},
		{
			"a unique path depends on its one module",
			[]Module{{Dir: "one", Path: "ex/dup"}, user},
			[]string{"one"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, unit := range Resolve(testCase.modules) {
				if unit.Dir == "user" && !slices.Equal(depDirs(unit), testCase.want) {
					t.Fatalf("deps of user = %v, want %v", depDirs(unit), testCase.want)
				}
			}
		})
	}
}

func TestResolveFoldsLocalDirectories(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		modules  []Module
		wantDeps map[string][]string
	}{
		{
			"a local that is a module brings its own requirements",
			[]Module{
				{Dir: "app", Path: "ex/app", Locals: []Local{{Dir: "lib", Subtree: "l"}}},
				{Dir: "lib", Path: "ex/lib", Requires: []string{"ex/base"}},
				{Dir: "base", Path: "ex/base"},
			},
			map[string][]string{"app": {"base", "lib"}, "base": {}, "lib": {"base"}},
		},
		{
			"a local that is no module is still a dependency",
			[]Module{
				{
					Dir: "app", Path: "ex/app",
					Locals: []Local{{Dir: "vendored/x", Subtree: "x"}, {Dir: "app", Subtree: "a"}},
				},
			},
			map[string][]string{"app": {"vendored/x"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, unit := range Resolve(testCase.modules) {
				if want := testCase.wantDeps[unit.Dir]; !slices.Equal(depDirs(unit), want) {
					t.Fatalf("deps of %s = %v, want %v", unit.Dir, depDirs(unit), want)
				}
			}
		})
	}
}

func TestResolveKeysReactToDependencyChange(t *testing.T) {
	t.Parallel()

	build := func(alphaTree string) map[string]string {
		units := Resolve([]Module{
			{Dir: "alpha", Path: "ex/alpha", Subtree: alphaTree},
			{Dir: "beta", Path: "ex/beta", Subtree: "b", Requires: []string{"ex/alpha"}},
			{Dir: "solo", Path: "ex/solo", Subtree: "c"},
		})
		keys := map[string]string{}

		for _, unit := range units {
			keys[unit.Dir] = Key(unit, "id")
		}

		return keys
	}

	before, after := build("one"), build("two")

	testCases := []struct {
		name        string
		dir         string
		wantChanged bool
	}{
		{"the changed module", "alpha", true},
		{"a module depending on it", "beta", true},
		{"an unrelated module is a cache hit", "solo", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if changed := before[testCase.dir] != after[testCase.dir]; changed != testCase.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, testCase.wantChanged)
			}
		})
	}
}
