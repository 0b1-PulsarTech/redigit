package entities

import "testing"

func TestLayersLayerOf(t *testing.T) {
	t.Parallel()

	layers := NewLayers([]LayerRule{
		{Prefix: "libs", Layer: 3},
		{Prefix: "libs/tereckernel", Layer: 1},
		{Prefix: "libs/infra", Layer: 6},
		{Prefix: "database", Layer: 4},
	})

	testCases := []struct {
		name string
		path string
		want Layer
	}{
		{
			name: "longest prefix wins over a shorter one that also matches",
			path: "libs/tereckernel/lead.go",
			want: 1,
		},
		{
			name: "a sibling under the same shorter prefix takes its own rule",
			path: "libs/infra/repo.go",
			want: 6,
		},
		{
			name: "a path only the shorter prefix covers takes the shorter rule",
			path: "libs/engines/flow.go",
			want: 3,
		},
		{
			name: "a prefix matching the path exactly",
			path: "database",
			want: 4,
		},
		{
			name: "no rule matches",
			path: "cmd/app/main.go",
			want: NoLayer,
		},
		{
			// "libs" must not claim "libsomething": a prefix names a directory,
			// and half an element is not a directory.
			name: "a prefix must match whole path elements",
			path: "libsomething/lead.go",
			want: NoLayer,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := layers.LayerOf(testCase.path); got != testCase.want {
				t.Fatalf("LayerOf(%q) = %d, want %d", testCase.path, got, testCase.want)
			}
		})
	}
}

func TestLayersEmpty(t *testing.T) {
	t.Parallel()

	if !(Layers{}).Empty() {
		t.Fatal("the zero Layers reports itself non-empty")
	}

	if NewLayers([]LayerRule{{Prefix: "libs", Layer: 1}}).Empty() {
		t.Fatal("a Layers holding one rule reports itself empty")
	}
}

// Rules must hand back a copy: a caller that sorts or truncates what it gets
// would otherwise silently reorder the matching inside LayerOf.
func TestLayersRulesReturnsACopy(t *testing.T) {
	t.Parallel()

	layers := NewLayers([]LayerRule{{Prefix: "libs", Layer: 3}, {Prefix: "cmd", Layer: 8}})

	rules := layers.Rules()
	rules[0] = LayerRule{Prefix: "clobbered", Layer: 99}

	if got := layers.LayerOf("libs/x.go"); got != 3 {
		t.Fatalf("LayerOf after mutating the returned slice = %d, want 3", got)
	}
}
