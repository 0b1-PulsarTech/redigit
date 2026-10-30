package lineage

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestShinglesAt(t *testing.T) {
	t.Parallel()

	src := []byte("one two three four")

	testCases := []struct {
		name      string
		span      entities.Span
		wantEmpty bool
	}{
		{"a span inside the source", entities.Span{Start: 0, End: 18}, false},
		{"a negative start", entities.Span{Start: -1, End: 4}, true},
		{"an end past the source", entities.Span{Start: 0, End: 99}, true},
		{"an empty span", entities.Span{Start: 4, End: 4}, true},
		{"an inverted span", entities.Span{Start: 8, End: 4}, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := shinglesAt(src, []entities.Decl{{Span: testCase.span}})
			if len(got) != 1 || (len(got[0]) == 0) != testCase.wantEmpty {
				t.Fatalf("shinglesAt = %v, wantEmpty %v", got, testCase.wantEmpty)
			}
		})
	}
}

func TestRankPairs(t *testing.T) {
	t.Parallel()

	candidateOf := func(from, to string, oldIdx, newIdx, score int) candidate {
		return candidate{
			declPair: declPair{from: key("a.go", from), to: key("b.go", to), textScore: score},
			oldIdx:   oldIdx, newIdx: newIdx,
		}
	}

	testCases := []struct {
		name  string
		found []candidate
		want  []declPair
	}{
		{"no candidates", nil, nil},
		{
			"the higher score wins the shared old declaration",
			[]candidate{candidateOf("X", "P", 0, 0, 70), candidateOf("X", "Q", 0, 1, 90)},
			[]declPair{{from: key("a.go", "X"), to: key("b.go", "Q"), textScore: 90}},
		},
		{
			"equal scores fall back to key order",
			[]candidate{
				candidateOf("Y", "P", 1, 0, 80), candidateOf("X", "P", 0, 0, 80),
				candidateOf("X", "Q", 0, 1, 80), candidateOf("Y", "Q", 1, 1, 80),
			},
			[]declPair{
				{from: key("a.go", "X"), to: key("b.go", "P"), textScore: 80},
				{from: key("a.go", "Y"), to: key("b.go", "Q"), textScore: 80},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := rankPairs(testCase.found)
			if len(got) != len(testCase.want) {
				t.Fatalf("rankPairs = %v, want %v", got, testCase.want)
			}

			for idx := range got {
				if got[idx] != testCase.want[idx] {
					t.Fatalf("rankPairs[%d] = %v, want %v", idx, got[idx], testCase.want[idx])
				}
			}
		})
	}
}
