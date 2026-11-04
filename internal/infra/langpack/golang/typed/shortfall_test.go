package typed

import (
	"context"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestTypedSourceKeepsAPartialPassAsData(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		files      map[string]string
		wantBroken bool
	}{
		{
			"every package checks",
			map[string]string{"a.go": "package fixture\n\nfunc One() {}\n"},
			false,
		},
		{
			"a package does not type-check",
			map[string]string{"a.go": "package fixture\n\nvar Bad int = \"text\"\n"},
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, decls := testkit.Fixture(t, testCase.files)
			source := NewSource([]string{"integration"})

			if got := source.Shortfalls(); len(got) != 0 {
				t.Fatalf("Shortfalls() = %+v before any load", got)
			}

			if _, err := source.Facts(context.Background(), snap, decls); err != nil {
				t.Fatalf("Facts() error = %v", err)
			}

			got := source.Shortfalls()
			if (len(got) > 0) != testCase.wantBroken {
				t.Fatalf("Shortfalls() = %+v, want broken = %v", got, testCase.wantBroken)
			}

			if testCase.wantBroken &&
				(got[0].Broken < 1 || got[0].Of < got[0].Broken ||
					!slices.Equal(got[0].Tags, []string{"integration"})) {
				t.Fatalf("Shortfalls() = %+v, want the broken count, total and tags", got)
			}
		})
	}
}

func TestTypedSourceKeepsTheUsesOfASoundPackageBesideABrokenDependency(t *testing.T) {
	t.Parallel()

	snap, decls := testkit.Fixture(t, map[string]string{
		"lib/lib.go": "package lib\n\nfunc One() {}\n",
		"bad/bad.go": "package bad\n\nfunc Broken() int { return \"s\" }\n",
		"a.go": "package fixture\n\nimport (\n\t\"example.test/fixture/bad\"\n\t" +
			"\"example.test/fixture/lib\"\n)\n\nfunc Run() {\n\tlib.One()\n\t_ = bad.Broken()\n}\n",
	})

	source := NewSource(nil)

	got, err := source.Facts(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Facts() error = %v", err)
	}

	run := testkit.DeclKey("a.go", entities.KindFunc, "Run")

	testCases := []struct {
		name     string
		provider entities.DeclKey
	}{
		{
			name:     "a use of the sound package",
			provider: testkit.DeclKey("lib/lib.go", entities.KindFunc, "One"),
		},
		{
			name:     "a use of the broken package",
			provider: testkit.DeclKey("bad/bad.go", entities.KindFunc, "Broken"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !slices.Contains(got.Uses, Use{Dependent: run, Provider: testCase.provider}) {
				t.Fatalf("Uses = %v, want Run to use %v", got.Uses, testCase.provider)
			}
		})
	}

	if short := source.Shortfalls(); len(short) != 1 || short[0].Broken != 1 {
		t.Fatalf("Shortfalls() = %+v, want the one broken package", short)
	}
}
