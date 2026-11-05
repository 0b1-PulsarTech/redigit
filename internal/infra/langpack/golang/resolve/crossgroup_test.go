package resolve

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

func TestTypedImplementationsAcrossGroups(t *testing.T) {
	t.Parallel()

	cell := map[string]string{
		"a.go": "package fixture\n\ntype Cell struct{}\n\nfunc (Cell) Do() error { return nil }\n\n" +
			"type Sink interface{ Put() int }\n",
	}

	with := func(name, content string) map[string]string {
		files := maps.Clone(cell)
		files[name] = content

		return files
	}

	testCases := []struct {
		name    string
		files   map[string]string
		query   spot
		want    []spot
		wantErr error
	}{
		{
			"a testdata interface a type of the main module meets",
			with("testdata/s/s.go", "package s\n\ntype Source interface{ Do() error }\n"),
			spot{"testdata/s/s.go", "Source", 0},
			nil,
			ErrNotCovered,
		},
		{
			"a main interface a type of testdata meets",
			with(
				"testdata/k/k.go",
				"package k\n\ntype Bin struct{}\n\nfunc (Bin) Put() int { return 0 }\n",
			),
			spot{"a.go", "Sink", 0},
			nil,
			ErrNotCovered,
		},
		{
			"a testdata interface with a method no type of the main module has",
			with("testdata/s/s.go", "package s\n\ntype Odd interface{ Nothing() }\n"),
			spot{"testdata/s/s.go", "Odd", 0},
			nil,
			nil,
		},
		{
			"a testdata interface whose method takes another number of arguments",
			with("testdata/s/s.go", "package s\n\ntype Arg interface{ Do(int) error }\n"),
			spot{"testdata/s/s.go", "Arg", 0},
			nil,
			nil,
		},
		{
			"a main interface with no other group",
			cell,
			spot{"a.go", "Sink", 0},
			nil,
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := newFixture(t, testCase.files)

			got, err := answerOf(fix.resolver, askImplementations, fix.locate(t, testCase.query))
			if !errors.Is(err, testCase.wantErr) || (testCase.wantErr == nil && err != nil) {
				t.Fatalf("Implementations() error = %v, want %v", err, testCase.wantErr)
			}

			want := fix.locations(t, testCase.want)
			if !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
				t.Fatalf("Implementations() = %v, want %v", got, want)
			}
		})
	}
}

func TestTypedImplementationsAcrossACgoGroup(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		cgo     bool
		wantErr error
	}{
		{"a group holding a cgo package", true, ErrNotCovered},
		{"a group without one", false, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			resolver := &Typed{}
			_, _ = resolver.built.Do(
				context.Background(),
				func(context.Context) ([]index, error) {
					return []index{{
						covered: map[string]covering{
							"a.go": {textFile: newTextFile([]byte("package a\n"))},
						},
						cgo: testCase.cgo,
					}}, nil
				},
			)

			_, err := resolver.Implementations(
				context.Background(), entities.Location{File: "a.go", Char: 8},
			)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Implementations() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestTypedKeepsTheAdhocFailure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		removed   bool
		cancelled bool
		wantErr   bool
	}{
		{"an ad hoc pass that fails", true, false, true},
		{"an ad hoc pass that succeeds", false, false, false},
		{"an ad hoc pass cut short by a cancelled build", false, true, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			snap, _ := testkit.Fixture(t, map[string]string{"a.go": "package fixture\n"})
			source := typed.NewSource(nil)

			if _, err := source.Packages(context.Background(), snap); err != nil {
				t.Fatalf("Packages() error = %v", err)
			}

			if testCase.removed {
				if err := os.Remove(filepath.Join(snap.Root, "go.mod")); err != nil {
					t.Fatalf("remove go.mod: %v", err)
				}
			}

			resolver := NewTyped(source, snap)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if testCase.cancelled {
				cancel()
			}

			_, _ = resolver.build(ctx)

			if got := resolver.AdhocErr() != nil; got != testCase.wantErr {
				t.Fatalf("AdhocErr() = %v, want an error %v", resolver.AdhocErr(), testCase.wantErr)
			}
		})
	}
}
