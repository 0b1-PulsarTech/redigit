package idiom

import (
	"testing"
)

func TestComponents(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		adjacent [][]int
		together [][]int
		apart    [][]int
	}{
		{
			name:     "a chain has one component per node",
			adjacent: [][]int{{1}, {2}, nil},
			apart:    [][]int{{0, 1}, {1, 2}, {0, 2}},
		},
		{
			name:     "a loop is one component",
			adjacent: [][]int{{1}, {2}, {0}},
			together: [][]int{{0, 1}, {1, 2}},
		},
		{
			name:     "two loops joined by one edge stay apart",
			adjacent: [][]int{{1}, {0, 2}, {3}, {2}},
			together: [][]int{{0, 1}, {2, 3}},
			apart:    [][]int{{1, 2}},
		},
		{
			name:     "a node with no edge is alone",
			adjacent: [][]int{nil, {1}},
			apart:    [][]int{{0, 1}},
		},
		{
			name:     "no nodes",
			adjacent: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			comp := components(testCase.adjacent)
			if len(comp) != len(testCase.adjacent) {
				t.Fatalf("components() = %v, want one entry per node", comp)
			}

			for _, pair := range testCase.together {
				if comp[pair[0]] != comp[pair[1]] {
					t.Fatalf("nodes %v are apart in %v", pair, comp)
				}
			}

			for _, pair := range testCase.apart {
				if comp[pair[0]] == comp[pair[1]] {
					t.Fatalf("nodes %v are together in %v", pair, comp)
				}
			}
		})
	}
}
