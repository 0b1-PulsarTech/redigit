package rewriting

import (
	"errors"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type messagesCase struct {
	name       string
	rows       []Row
	subjects   map[string]string
	allowDraft bool
	want       []string
	wantErr    error
}

func runMessages(t *testing.T, testCases []messagesCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Messages(testCase.rows, testCase.subjects, testCase.allowDraft)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Messages() err = %v, want %v", err, testCase.wantErr)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("Messages() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestMessages(t *testing.T) {
	t.Parallel()

	draft := entities.DraftMarker + "feat: x"

	testCases := []messagesCase{
		{
			name:     "override wins and missing key keeps the row",
			rows:     []Row{{Position: 0, Subject: "a"}, {Position: 1, Subject: "b"}},
			subjects: map[string]string{"0": "override"},
			want:     []string{"override\n", "b\n"},
		},
		{
			name: "trailing newlines collapse to one",
			rows: []Row{{Position: 0, Subject: "a\n\n\n"}},
			want: []string{"a\n"},
		},
		{
			name:    "draft refused",
			rows:    []Row{{Position: 0, Subject: draft}},
			wantErr: apperr.ErrIncomplete,
		},
		{
			name:       "draft allowed",
			rows:       []Row{{Position: 0, Subject: draft}},
			allowDraft: true,
			want:       []string{draft + "\n"},
		},
		{
			name:     "override can clear a draft",
			rows:     []Row{{Position: 0, Subject: draft}},
			subjects: map[string]string{"0": "feat: y"},
			want:     []string{"feat: y\n"},
		},
		{
			name:     "override of a draft is still refused",
			rows:     []Row{{Position: 0, Subject: "a"}},
			subjects: map[string]string{"0": draft},
			wantErr:  apperr.ErrIncomplete,
		},
	}

	runMessages(t, testCases)
}

func TestMessagesKeepsTheWIPMarker(t *testing.T) {
	t.Parallel()

	wipRow := Row{Position: 0, Subject: ":gem: feat(a): [wip] Gen", IsWIP: true, ID: "aa11"}
	marked := ":bug: fix(b): [wip] Hand\n"

	testCases := []messagesCase{
		{
			name:     "hand subject on a wip row keeps the marker",
			rows:     []Row{wipRow},
			subjects: map[string]string{"0": ":bug: fix(b): Hand"},
			want:     []string{marked},
		},
		{
			name:     "a marker already written by hand is not doubled",
			rows:     []Row{wipRow},
			subjects: map[string]string{"0": ":bug: fix(b): [wip] Hand"},
			want:     []string{marked},
		},
		{
			name:     "a marker in the words is text and the real one is added",
			rows:     []Row{wipRow},
			subjects: map[string]string{"0": ":gem: feat(b): Keep the [wip] marker in docs"},
			want:     []string{":gem: feat(b): [wip] Keep the [wip] marker in docs\n"},
		},
		{
			name:     "a marker in the words on a non wip row is written as given",
			rows:     []Row{{Position: 0, Subject: "a"}},
			subjects: map[string]string{"0": ":gem: feat(b): Keep the [wip] marker in docs"},
			want:     []string{":gem: feat(b): Keep the [wip] marker in docs\n"},
		},
		{
			name:     "capitals are text and the real marker is added",
			rows:     []Row{wipRow},
			subjects: map[string]string{"0": ":gem: feat(b): [WIP] One"},
			want:     []string{":gem: feat(b): [wip] [WIP] One\n"},
		},
		{
			name:     "a marker on a non wip row is written as given",
			rows:     []Row{{Position: 0, Subject: "a"}},
			subjects: map[string]string{"0": ":bug: fix(b): [wip] Hand"},
			want:     []string{":bug: fix(b): [wip] Hand\n"},
		},
		{
			name:     "multi-line value keeps the marker on line one only",
			rows:     []Row{wipRow},
			subjects: map[string]string{"0": ":bug: fix(b): Hand\n\nwhy: a reason\n"},
			want:     []string{":bug: fix(b): [wip] Hand\n\nwhy: a reason\n"},
		},
		{
			name:     "a wip row with no hand subject keeps the one it has",
			rows:     []Row{wipRow},
			want:     []string{":gem: feat(a): [wip] Gen\n"},
			subjects: nil,
		},
	}

	runMessages(t, testCases)
}

func TestMessagesRefusesBadKeys(t *testing.T) {
	t.Parallel()

	wipRow := Row{Position: 0, Subject: ":gem: feat(a): [wip] Gen", IsWIP: true, ID: "aa11"}

	testCases := []messagesCase{
		{
			name:     "an id key reaches its row and a wip row keeps the marker",
			rows:     []Row{wipRow, {Position: 1, Subject: "b", ID: "bb22"}},
			subjects: map[string]string{"id:aa11": ":bug: fix(b): Hand"},
			want:     []string{":bug: fix(b): [wip] Hand\n", "b\n"},
		},
		{
			name:     "an unknown id key is refused",
			rows:     []Row{wipRow},
			subjects: map[string]string{"id:ffff": "x"},
			wantErr:  ErrSubjects,
		},
		{
			name:     "position and id for the same row are refused",
			rows:     []Row{wipRow},
			subjects: map[string]string{"0": "x", "id:aa11": "y"},
			wantErr:  ErrSubjects,
		},
		{
			name:     "a position outside the manifest is refused",
			rows:     []Row{wipRow},
			subjects: map[string]string{"7": "x"},
			wantErr:  ErrSubjects,
		},
		{
			name:     "a key that is neither position nor id is refused",
			rows:     []Row{wipRow},
			subjects: map[string]string{"first": "x"},
			wantErr:  ErrSubjects,
		},
	}

	runMessages(t, testCases)
}
