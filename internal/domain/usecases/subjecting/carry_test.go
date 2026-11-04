package subjecting

import (
	"slices"
	"testing"
)

type carryCase struct {
	name     string
	old      []Entry
	subjects map[string]string
	current  []Entry
	want     []int
}

func runCarry(t *testing.T, testCases []carryCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Carry(testCase.old, testCase.subjects, testCase.current)
			counts := []int{
				len(got.Identical), len(got.Review), len(got.Lost), got.Fresh, len(got.Problems),
			}

			if !slices.Equal(counts, testCase.want) {
				t.Fatalf("Carry() = %v, want %v (identical, review, lost, fresh, problems)",
					counts, testCase.want)
			}
		})
	}
}

func TestCarryIdentical(t *testing.T) {
	t.Parallel()

	testCases := []carryCase{
		{
			name: "tier one: an identical chunk set carries",
			old: []Entry{
				{Position: 0, ID: ID([]string{"a", "b"}, nil), Chunks: []string{"a", "b"}},
			},
			subjects: map[string]string{"0": "hand"},
			current: []Entry{
				{Position: 4, ID: ID([]string{"b", "a"}, nil), Chunks: []string{"b", "a"}},
			},
			want: []int{1, 0, 0, 0, 0},
		},
	}

	runCarry(t, testCases)
}

func TestCarryEdges(t *testing.T) {
	t.Parallel()

	hand := map[string]string{"0": "hand"}
	chunkA := Entry{Position: 0, ID: ID([]string{"a"}, nil), Chunks: []string{"a"}}

	testCases := []carryCase{
		{
			name:    "an old row without a hand subject is not a source",
			old:     []Entry{chunkA},
			current: []Entry{chunkA},
			want:    []int{0, 0, 0, 1, 0},
		},
		{
			name:     "an id keyed old subject carries",
			old:      []Entry{chunkA},
			subjects: map[string]string{IDPrefix + ID([]string{"a"}, nil): "hand"},
			current:  []Entry{{Position: 9, ID: ID([]string{"a"}, nil), Chunks: []string{"a"}}},
			want:     []int{1, 0, 0, 0, 0},
		},
		{
			name:     "a stale old key is reported as a problem",
			old:      []Entry{chunkA},
			subjects: map[string]string{"9": "hand"},
			current:  []Entry{chunkA},
			want:     []int{0, 0, 0, 1, 1},
		},
		{
			name:     "a signed old key is reported as a problem",
			old:      []Entry{chunkA},
			subjects: map[string]string{"-0": "hand"},
			current:  []Entry{chunkA},
			want:     []int{0, 0, 0, 1, 1},
		},
		{
			name: "empty chunks compare by blob paths",
			old: []Entry{
				{Position: 0, ID: "old", Blobs: []string{"a", "b", "c", "d", "e"}},
			},
			subjects: hand,
			current: []Entry{
				{Position: 3, ID: "new", Blobs: []string{"a", "b", "c", "d", "e", "f"}},
			},
			want: []int{0, 1, 0, 0, 0},
		},
		{
			name:     "a row with nothing to compare is lost",
			old:      []Entry{{Position: 0, ID: ID(nil, nil)}},
			subjects: hand,
			current:  []Entry{{Position: 0, ID: "other"}},
			want:     []int{0, 0, 1, 1, 0},
		},
		{
			name: "identical ids on the new side are ambiguous and fall to tier two",
			old: []Entry{
				{Position: 0, ID: ID([]string{"a", "b"}, nil), Chunks: []string{"a", "b"}},
			},
			subjects: hand,
			current: []Entry{
				{Position: 1, ID: ID([]string{"a", "b"}, nil), Chunks: []string{"a", "b"}},
				{Position: 2, ID: ID([]string{"a", "b"}, nil), Chunks: []string{"a", "b"}},
			},
			want: []int{0, 0, 1, 2, 0},
		},
	}

	runCarry(t, testCases)
}

func TestCarryReportsTheMatch(t *testing.T) {
	t.Parallel()

	five := []string{"a", "b", "c", "d", "e"}
	six := []string{"a", "b", "c", "d", "e", "f"}

	testCases := []struct {
		name     string
		old      []Entry
		subjects map[string]string
		current  []Entry
		want     Match
	}{
		{
			name:     "a near match is review and keeps the hand subject",
			old:      []Entry{{Position: 2, ID: ID(five, nil), Chunks: five}},
			subjects: map[string]string{"2": "hand"},
			current:  []Entry{{Position: 8, ID: "new", Chunks: six}},
			want:     Match{From: 2, To: 8, ID: "new", Tier: TierReview, Subject: "hand"},
		},
		{
			name:     "a marker in place is stripped from a review subject",
			old:      []Entry{{Position: 2, ID: ID(five, nil), Chunks: five}},
			subjects: map[string]string{"2": ":bug: fix(a): [wip] Hand"},
			current:  []Entry{{Position: 8, ID: "new", Chunks: six}},
			want: Match{
				From: 2, To: 8, ID: "new", Tier: TierReview, Subject: ":bug: fix(a): Hand",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Carry(testCase.old, testCase.subjects, testCase.current)
			if len(got.Review) != 1 {
				t.Fatalf("Carry() review = %+v", got.Review)
			}

			match := got.Review[0]
			if match.Score < SimilarityFloor || match.Score >= 1 {
				t.Fatalf("Carry() score = %v", match.Score)
			}

			match.Score = 0
			if match != testCase.want {
				t.Fatalf("Carry() match = %+v, want %+v", match, testCase.want)
			}
		})
	}
}

func TestCarryStripsTheMarker(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		subject string
		want    string
	}{
		{name: "in place", subject: ":gem: feat(a): [wip] Hand", want: ":gem: feat(a): Hand"},
		{
			name:    "after a draft",
			subject: "DRAFT: feat(a): [wip] Hand",
			want:    "DRAFT: feat(a): Hand",
		},
		{name: "none", subject: ":gem: feat(a): Hand", want: ":gem: feat(a): Hand"},
		{name: "doubled", subject: ":bug: fix(b): [wip] [wip] Two", want: ":bug: fix(b): Two"},
		{name: "ahead of the type", subject: ":bug: [wip] fix(b): Old", want: ":bug: fix(b): Old"},
		{
			name:    "ahead of the emoji",
			subject: "[wip] :bug: fix(b): Lead",
			want:    ":bug: fix(b): Lead",
		},
		{
			name:    "spaced scope",
			subject: ":bug: fix(my scope): [wip] Space",
			want:    ":bug: fix(my scope): Space",
		},
		{
			name:    "closing line one",
			subject: ":bug: fix(b): [wip]\nHand",
			want:    ":bug: fix(b): Hand",
		},
		{
			name:    "words stay",
			subject: ":gem: feat(a): Keep the [wip] marker",
			want:    ":gem: feat(a): Keep the [wip] marker",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			entry := Entry{Position: 0, ID: ID([]string{"a"}, nil), Chunks: []string{"a"}}
			got := Carry([]Entry{entry}, map[string]string{"0": testCase.subject}, []Entry{entry})

			if len(got.Identical) != 1 || got.Identical[0].Subject != testCase.want {
				t.Fatalf("Carry() identical = %+v, want subject %q", got.Identical, testCase.want)
			}
		})
	}
}
