package anchorprobe

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func TestTrialled(t *testing.T) {
	t.Parallel()

	root := verifying.Unit{Module: verifying.Module{Dir: "."}}
	nested := verifying.Unit{Module: verifying.Module{Dir: "sub"}}
	rooted := verifying.Unit{
		Module: verifying.Module{Dir: "."}, Deps: []verifying.Module{{Dir: "sub"}},
	}
	dependent := verifying.Unit{
		Module: verifying.Module{Dir: "sub"}, Deps: []verifying.Module{{Dir: "."}},
	}

	testCases := []struct {
		name     string
		units    []verifying.Unit
		exclude  []string
		files    []string
		wantKept []string
		wantLeft []string
	}{
		{
			name:     "no exclusion keeps every unit",
			units:    []verifying.Unit{root, nested},
			wantKept: []string{".", "sub"},
		},
		{
			name:     "an excluded nested module is left while the root is trialled",
			units:    []verifying.Unit{root, nested},
			exclude:  []string{"sub"},
			files:    []string{"sub/s.go"},
			wantKept: []string{"."}, wantLeft: []string{"sub"},
		},
		{
			name:     "an excluded module the root requires is left when the edit is in it",
			units:    []verifying.Unit{rooted, nested},
			exclude:  []string{"sub"},
			files:    []string{"sub/other/o.go"},
			wantKept: []string{"."}, wantLeft: []string{"sub"},
		},
		{
			name:     "an excluded module that only depends on the edited root is not named",
			units:    []verifying.Unit{root, dependent},
			exclude:  []string{"sub"},
			files:    []string{"a.go"},
			wantKept: []string{"."},
		},
		{
			name:     "only excluded units leaves nothing to trial",
			units:    []verifying.Unit{nested},
			exclude:  []string{"sub"},
			files:    []string{"sub/s.go"},
			wantLeft: []string{"sub"},
		},
		{
			name:    "no unit at all leaves nothing to either side",
			exclude: []string{"sub"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			kept, left := trialled(testCase.units, testCase.exclude, testCase.files)

			if got := slices.Collect(dirsIn(kept)); !slices.Equal(got, testCase.wantKept) {
				t.Fatalf("kept = %v, want %v", got, testCase.wantKept)
			}

			if got := slices.Collect(dirsIn(left)); !slices.Equal(got, testCase.wantLeft) {
				t.Fatalf("left = %v, want %v", got, testCase.wantLeft)
			}
		})
	}
}

func dirsIn(units []verifying.Unit) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for _, unit := range units {
			if !yield(unit.Dir) {
				return
			}
		}
	}
}

func TestUncheckedBy(t *testing.T) {
	t.Parallel()

	root := []verifying.Unit{{Module: verifying.Module{Dir: "."}}}
	nested := []verifying.Unit{{Module: verifying.Module{Dir: "sub"}}}

	testCases := []struct {
		name          string
		verdict       anchoring.Verdict
		left          []verifying.Unit
		wantUnchecked bool
		wantDetail    string
	}{
		{
			name:    "a built verdict with nothing left out stays built",
			verdict: anchoring.Verdict{IsBuilt: true},
		},
		{
			name:          "a built verdict with an excluded owner reads unchecked and says what ran",
			verdict:       anchoring.Verdict{IsBuilt: true},
			left:          nested,
			wantUnchecked: true,
			wantDetail:    "built .; excluded by verify.exclude and not built: sub",
		},
		{
			name:       "a failure stays a failure whatever was left out",
			verdict:    anchoring.Verdict{Detail: "undefined: g"},
			left:       nested,
			wantDetail: "undefined: g",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := uncheckedBy(testCase.verdict, root, testCase.left)

			if got.IsUnchecked != testCase.wantUnchecked || got.Detail != testCase.wantDetail {
				t.Fatalf("uncheckedBy() = %+v, want unchecked %v with %q",
					got, testCase.wantUnchecked, testCase.wantDetail)
			}
		})
	}
}
