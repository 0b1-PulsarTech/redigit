package subjecting

import "testing"

func TestCarrySimilar(t *testing.T) {
	t.Parallel()

	hand := map[string]string{"0": "hand"}

	testCases := []carryCase{
		{
			name: "tier two: a near match is review, never carried",
			old: []Entry{
				{Position: 0, ID: "old", Chunks: []string{"a", "b", "c", "d", "e"}},
			},
			subjects: hand,
			current: []Entry{
				{Position: 7, ID: "new", Chunks: []string{"a", "b", "c", "d", "e", "f"}},
			},
			want: []int{0, 1, 0, 0, 0},
		},
		{
			name:     "tier three: below the floor stays unmatched",
			old:      []Entry{{Position: 0, ID: "old", Chunks: []string{"a", "b", "c", "d"}}},
			subjects: hand,
			current:  []Entry{{Position: 1, ID: "new", Chunks: []string{"a", "x", "y", "z"}}},
			want:     []int{0, 0, 1, 1, 0},
		},
		{
			name:     "two equal candidates give no match",
			old:      []Entry{{Position: 0, ID: "old", Chunks: []string{"a", "b", "c", "d"}}},
			subjects: hand,
			current: []Entry{
				{Position: 1, ID: "n1", Chunks: []string{"a", "b", "c", "d", "e"}},
				{Position: 2, ID: "n2", Chunks: []string{"a", "b", "c", "d", "f"}},
			},
			want: []int{0, 0, 1, 2, 0},
		},
		{
			name: "a clearly better candidate wins over a worse one",
			old: []Entry{
				{
					Position: 0, ID: "old",
					Chunks: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"},
				},
			},
			subjects: hand,
			current: []Entry{
				{
					Position: 1, ID: "n1",
					Chunks: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
				},
				{
					Position: 2, ID: "n2",
					Chunks: []string{"a", "b", "c", "d", "e", "f", "g", "h", "x", "y"},
				},
			},
			want: []int{0, 1, 0, 1, 0},
		},
		{
			name: "two old rows wanting one new row both lose",
			old: []Entry{
				{Position: 0, ID: "o0", Chunks: []string{"a", "b", "c", "d", "e"}},
				{Position: 1, ID: "o1", Chunks: []string{"a", "b", "c", "d", "f"}},
			},
			subjects: map[string]string{"0": "x", "1": "y"},
			current: []Entry{
				{Position: 5, ID: "n5", Chunks: []string{"a", "b", "c", "d", "e", "f"}},
			},
			want: []int{0, 0, 2, 1, 0},
		},
	}

	runCarry(t, testCases)
}
