package anchorprobe

import (
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func TestAffectedBy(t *testing.T) {
	t.Parallel()

	both := []string{"apps/web", "libs/core"}
	rootOnly := []verifying.Unit{{Module: verifying.Module{Dir: "."}}}
	nested := verifying.Module{Dir: "apps/web"}
	lib := verifying.Module{Dir: "libs/core"}
	units := []verifying.Unit{
		{Module: nested, Deps: []verifying.Module{lib}},
		{Module: lib},
		{Module: verifying.Module{Dir: "tools"}},
	}

	testCases := []struct {
		name  string
		units []verifying.Unit
		files []string
		want  []string
	}{
		{
			name:  "a file of a dependency",
			units: units, files: []string{"libs/core/a.go"}, want: both,
		},
		{
			name:  "a file only the dependent holds",
			units: units, files: []string{"apps/web/a.go"}, want: []string{"apps/web"},
		},
		{
			name:  "a file no module holds",
			units: units, files: []string{"docs/readme.md"},
		},
		{
			name:  "a root module holds every file",
			units: rootOnly, files: []string{"x/y.go"}, want: []string{"."},
		},
		{
			name: "a nested module owns its files and the root around it does not build them",
			units: []verifying.Unit{
				{Module: verifying.Module{Dir: "."}}, {Module: verifying.Module{Dir: "sub"}},
			},
			files: []string{"sub/s.go"}, want: []string{"sub"},
		},
		{
			name: "a root module that requires the nested one still builds its files",
			units: []verifying.Unit{
				{Module: verifying.Module{Dir: "."}, Deps: []verifying.Module{{Dir: "sub"}}},
				{Module: verifying.Module{Dir: "sub"}},
			},
			files: []string{"sub/s.go"}, want: []string{".", "sub"},
		},
		{
			name: "a file of the root is not a file of a nested module",
			units: []verifying.Unit{
				{Module: verifying.Module{Dir: "."}}, {Module: verifying.Module{Dir: "sub"}},
			},
			files: []string{"a.go"}, want: []string{"."},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var got []string

			for _, unit := range affectedBy(testCase.units, testCase.files) {
				got = append(got, unit.Dir)
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("affectedBy() = %v, want %v", got, testCase.want)
			}
		})
	}
}
