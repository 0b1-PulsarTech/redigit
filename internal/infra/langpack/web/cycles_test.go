package web

import (
	"maps"
	"testing"
)

func TestCycleAtoms(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		imports map[string][]string
		want    map[string]string
	}

	testCases := []testCase{
		{
			name:    "no cycle",
			imports: map[string][]string{"a": {"b"}, "b": {"c"}, "c": nil},
			want:    map[string]string{},
		},
		{
			name:    "a pair",
			imports: map[string][]string{"b": {"a"}, "a": {"b"}},
			want:    map[string]string{"a": "a", "b": "a"},
		},
		{
			name: "two cycles and a file between them",
			imports: map[string][]string{
				"x": {"y"}, "y": {"z"}, "z": {"x", "m"},
				"m": {"p"}, "p": {"q"}, "q": {"p"},
			},
			want: map[string]string{"x": "x", "y": "x", "z": "x", "p": "p", "q": "p"},
		},
		{
			name:    "a file importing itself",
			imports: map[string][]string{"a": {"a"}},
			want:    map[string]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := cycleAtoms(tc.imports); !maps.Equal(got, tc.want) {
				t.Errorf("cycleAtoms() = %v, want %v", got, tc.want)
			}
		})
	}
}
