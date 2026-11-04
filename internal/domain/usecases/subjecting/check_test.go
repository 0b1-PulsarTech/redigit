package subjecting

import (
	"slices"
	"strings"
	"testing"
)

type checkCase struct {
	name     string
	rows     []Row
	subjects map[string]string
	want     []Finding
}

func runCheck(t *testing.T, testCases []checkCase) {
	t.Helper()

	limits := Limits{Description: 20, Line: 40}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Check(testCase.rows, testCase.subjects, limits)
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("Check() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()

	good := ":bug: fix(a): Hand written"
	draft := "DRAFT: feat(a): x"

	testCases := []checkCase{
		{
			name:     "a clean hand subject passes",
			rows:     []Row{{Position: 0, Subject: draft}},
			subjects: map[string]string{"0": good},
		},
		{
			name: "a non draft manifest subject needs no work",
			rows: []Row{{Position: 0, Subject: "feat(a): Done"}},
		},
		{
			name: "a draft with no hand subject is uncovered",
			rows: []Row{{Position: 3, Subject: draft}},
			want: []Finding{{Position: 3, Kind: KindUncovered}},
		},
		{
			name:     "a hand subject still marked draft is refused",
			rows:     []Row{{Position: 0, Subject: draft}},
			subjects: map[string]string{"0": draft},
			want:     []Finding{{Position: 0, Kind: KindDraft}},
		},
		{
			name:     "a malformed subject is a format finding",
			rows:     []Row{{Position: 0, Subject: draft}},
			subjects: map[string]string{"0": "just words"},
			want:     []Finding{{Position: 0, Kind: KindFormat}},
		},
		{
			name: "a wip row and a plain row with the same words are a duplicate",
			rows: []Row{
				{Position: 0, Subject: draft},
				{Position: 1, Subject: draft, IsWIP: true},
			},
			subjects: map[string]string{"0": good, "1": good},
			want:     []Finding{{Position: 1, Kind: KindDuplicate, Key: "0"}},
		},
		{
			name: "two wip rows with the same words are a duplicate",
			rows: []Row{
				{Position: 0, Subject: draft, IsWIP: true},
				{Position: 1, Subject: draft, IsWIP: true},
			},
			subjects: map[string]string{"0": good, "1": good},
			want:     []Finding{{Position: 1, Kind: KindDuplicate, Key: "0"}},
		},
		{
			name: "the same subject twice is a duplicate",
			rows: []Row{
				{Position: 0, Subject: draft},
				{Position: 1, Subject: draft},
			},
			subjects: map[string]string{"0": good, "1": good},
			want:     []Finding{{Position: 1, Kind: KindDuplicate, Key: "0"}},
		},
	}

	runCheck(t, testCases)
}

func TestCheckLengths(t *testing.T) {
	t.Parallel()

	draft := "DRAFT: feat(a): x"

	testCases := []checkCase{
		{
			name:     "a long description is a description_length finding",
			rows:     []Row{{Position: 0, Subject: draft}},
			subjects: map[string]string{"0": ":bug: fix(a): aaaaaaaaaaaaaaaaaaaaa"},
			want:     []Finding{{Position: 0, Kind: KindDescriptionLength, Detail: "21"}},
		},
		{
			name:     "a long line is a line_length finding",
			rows:     []Row{{Position: 0, Subject: draft}},
			subjects: map[string]string{"0": ":bug: fix(aaaaaaaaaaaaaaaaaaaaaaaaaaaaa): short"},
			want:     []Finding{{Position: 0, Kind: KindLineLength, Detail: "47"}},
		},
		{
			name:     "the marker counts toward the written length",
			rows:     []Row{{Position: 0, Subject: draft, IsWIP: true}},
			subjects: map[string]string{"0": ":bug: fix(a): aaaaaaaaaaaaaaa"},
			want:     []Finding{{Position: 0, Kind: KindDescriptionLength, Detail: "21"}},
		},
	}

	runCheck(t, testCases)
}

func TestCheckMarkerAndKeys(t *testing.T) {
	t.Parallel()

	good := ":bug: fix(a): Hand written"
	draft := "DRAFT: feat(a): x"
	keyed := IDPrefix + ID([]string{"k"}, nil)

	testCases := []checkCase{
		{
			name:     "a marker on a non wip row is refused",
			rows:     []Row{{Position: 0, Subject: draft}},
			subjects: map[string]string{"0": ":bug: fix(a): [wip] Hand"},
			want:     []Finding{{Position: 0, Kind: KindWIPMarker}},
		},
		{
			name:     "a marker on a wip row is fine",
			rows:     []Row{{Position: 0, Subject: draft, IsWIP: true}},
			subjects: map[string]string{"0": ":bug: fix(a): [wip] Hand"},
		},
		{
			name:     "a stale position is reported",
			rows:     []Row{{Position: 0, Subject: "feat(a): Done"}},
			subjects: map[string]string{"4": good},
			want:     []Finding{{Position: NoPosition, Kind: KindStaleKey, Key: "4"}},
		},
		{
			name:     "an unknown id is reported",
			rows:     []Row{{Position: 0, Subject: "feat(a): Done"}},
			subjects: map[string]string{"id:zz": good},
			want:     []Finding{{Position: NoPosition, Kind: KindUnknownID, Key: "id:zz"}},
		},
		{
			name:     "position and id for one row conflict",
			rows:     []Row{{Position: 0, Subject: draft, ChunkKeys: []string{"k"}}},
			subjects: map[string]string{"0": good, keyed: ":gem: feat(b): Other"},
			want: []Finding{
				{Position: NoPosition, Kind: KindConflict, Key: "0," + keyed},
				{Position: 0, Kind: KindUncovered},
			},
		},
		{
			name:     "an id key reaches its row",
			rows:     []Row{{Position: 0, Subject: draft, ChunkKeys: []string{"k"}}},
			subjects: map[string]string{keyed: good},
		},
	}

	runCheck(t, testCases)
}

func TestCheckMarkerForms(t *testing.T) {
	t.Parallel()

	testCases := []struct{ name, subject string }{
		{name: "closing line one", subject: ":bug: fix(b): [wip]\nHand"},
		{name: "doubled", subject: ":bug: fix(b): [wip] [wip] Two"},
		{name: "ahead of the type", subject: ":bug: [wip] fix(b): Old"},
		{name: "ahead of the emoji", subject: "[wip] :bug: fix(b): Lead"},
		{name: "spaced scope", subject: ":bug: fix(my scope): [wip] Space"},
		{name: "with a body", subject: ":bug: fix(b): [wip] Hand\n\nbody"},
	}

	limits := Limits{Description: 20, Line: 40}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			subjects := map[string]string{"0": testCase.subject}
			clean := []Row{{Position: 0, Subject: "DRAFT: feat(a): x"}}
			wantClean := []Finding{{Position: 0, Kind: KindWIPMarker}}

			if got := Check(clean, subjects, limits); !slices.Equal(got, wantClean) {
				t.Fatalf("clean row: Check() = %+v, want %+v", got, wantClean)
			}

			wip := []Row{{Position: 0, Subject: "DRAFT: feat(a): x", IsWIP: true}}
			if got := Check(wip, subjects, limits); len(got) != 0 {
				t.Fatalf("wip row: Check() = %+v, want none", got)
			}
		})
	}
}

func TestCheckLimitsOffMeansUnlimited(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		limits Limits
		want   int
	}{
		{name: "both off", limits: Limits{}, want: 0},
		{name: "description on", limits: Limits{Description: 20}, want: 1},
		{name: "line on", limits: Limits{Line: 40}, want: 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rows := []Row{{Position: 0, Subject: "DRAFT: feat(a): x"}}
			subjects := map[string]string{"0": ":bug: fix(a): " + strings.Repeat("a", 200)}

			if got := Check(rows, subjects, testCase.limits); len(got) != testCase.want {
				t.Fatalf("Check() = %+v, want %d findings", got, testCase.want)
			}
		})
	}
}
