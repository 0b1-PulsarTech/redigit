package resolve

import (
	"errors"
	"maps"
	"slices"
	"testing"
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
