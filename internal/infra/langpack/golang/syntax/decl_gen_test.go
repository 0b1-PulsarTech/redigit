package syntax

import (
	"context"
	"testing"
)

func atomsByName(t *testing.T, source string) map[string]string {
	t.Helper()

	decls, err := Declarations(context.Background(), "sample.go", []byte(source))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	atomOf := map[string]string{}
	for _, decl := range decls {
		atomOf[decl.Key.Name] = decl.Atom
	}

	return atomOf
}

func assertGroups(t *testing.T, atomOf map[string]string, groups [][]string) {
	t.Helper()

	seen := map[string]bool{}

	for _, group := range groups {
		label := atomOf[group[0]]
		if label == "" || seen[label] {
			t.Fatalf("%s atom = %q, want one atom of its own (seen: %v)", group[0], label, seen)
		}

		seen[label] = true

		for _, name := range group {
			if atomOf[name] != label {
				t.Fatalf("%s atom = %q, want %q", name, atomOf[name], label)
			}
		}
	}
}

func TestBuildAtomicGroupSharesOneAtom(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		source     string
		wantGroups [][]string
	}{
		{
			name: "implicit spec members share one atom",
			source: `package sample

const (
	KeyA Key = iota + C01
	KeyB
	KeyC
)
`,
			wantGroups: [][]string{{"KeyA", "KeyB", "KeyC"}},
		},
		{
			name: "two implicit blocks in one file are two atoms",
			source: `package sample

const (
	One = iota
	Two
)

const (
	Red = iota
	Green
)
`,
			wantGroups: [][]string{{"One", "Two"}, {"Red", "Green"}},
		},
		{
			name: "two blocks that open with a blank get different atoms",
			source: `package sample

const (
	_ = iota
	Alpha
)

const (
	_ = iota
	Beta
)
`,
			wantGroups: [][]string{{"Alpha"}, {"Beta"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertGroups(t, atomsByName(t, testCase.source), testCase.wantGroups)
		})
	}
}

func TestBuildValueSpecsShareOneAtom(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		source     string
		wantGroups [][]string
	}{
		{
			name:       "a const spec binding two names is one atom",
			source:     "package sample\n\nconst KeyA, KeyB = 1, C01 + 1\n",
			wantGroups: [][]string{{"KeyA", "KeyB"}},
		},
		{
			name:       "a var spec binding two names is one atom",
			source:     "package sample\n\nvar VarA, VarB = 1, C01 + 1\n",
			wantGroups: [][]string{{"VarA", "VarB"}},
		},
		{
			name: "a multi-name spec inside a block is its own atom",
			source: `package sample

const (
	// KeyA doc
	KeyA, KeyB = 1, C01
	KeyC       = 3
)

var (
	VarA, VarB = 1, 2
	VarC, VarD = 3, 4
)
`,
			wantGroups: [][]string{{"KeyA", "KeyB"}, {"VarA", "VarB"}, {"VarC", "VarD"}},
		},
		{
			name: "explicit specs where a later one uses iota are one atom",
			source: `package sample

const (
	KeyA = iota + C01
	KeyB = iota
)
`,
			wantGroups: [][]string{{"KeyA", "KeyB"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assertGroups(t, atomsByName(t, testCase.source), testCase.wantGroups)
		})
	}
}

func TestBuildValueSpecsCarryNoAtom(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		source    string
		wantLoose []string
	}{
		{
			name: "explicit values carry no atom",
			source: `package sample

const (
	KeyA Key = 1
	KeyB Key = 2
)
`,
			wantLoose: []string{"KeyA", "KeyB"},
		},
		{
			name: "var block carries no atom",
			source: `package sample

var (
	First  = 1
	Second = 2
)
`,
			wantLoose: []string{"First", "Second"},
		},
		{
			name: "a single-name spec beside a multi-name one",
			source: `package sample

const (
	KeyA, KeyB = 1, C01
	KeyC       = 3
)
`,
			wantLoose: []string{"KeyC"},
		},
		{
			name: "iota in the first spec only",
			source: `package sample

const (
	KeyA = iota
	KeyB = 5
)
`,
			wantLoose: []string{"KeyA", "KeyB"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			atomOf := atomsByName(t, testCase.source)

			for _, name := range testCase.wantLoose {
				if atomOf[name] != "" {
					t.Fatalf("%s carries atom %q, want none", name, atomOf[name])
				}
			}
		})
	}
}
