package subjecting

import (
	"strings"
	"testing"
)

type markCase struct {
	name    string
	message string
	want    string
}

func runMark(t *testing.T, testCases []markCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Mark(testCase.message); got != testCase.want {
				t.Fatalf("Mark() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestMark(t *testing.T) {
	t.Parallel()

	testCases := []markCase{
		{
			name:    "a wip row gets the marker",
			message: ":bug: fix(a): x",
			want:    ":bug: fix(a): [wip] x",
		},
		{
			name:    "a seen marker is not doubled",
			message: ":bug: fix(a): [wip] x",
			want:    ":bug: fix(a): [wip] x",
		},
		{
			name:    "a marker in the words is text and is kept",
			message: ":gem: feat(b): Keep the [wip] marker in docs",
			want:    ":gem: feat(b): [wip] Keep the [wip] marker in docs",
		},
		{
			name:    "capitals are text",
			message: ":gem: feat(b): [WIP] One",
			want:    ":gem: feat(b): [wip] [WIP] One",
		},
		{name: "no emoji and a type", message: "fix(a): x", want: "fix(a): [wip] x"},
		{
			name:    "no emoji marker is seen",
			message: "fix(a): [wip] x",
			want:    "fix(a): [wip] x",
		},
		{name: "no colon at all", message: "plain", want: "[wip] plain"},
		{
			name:    "a leading marker is seen",
			message: "[wip] plain",
			want:    "[wip] plain",
		},
		{
			name:    "a draft prefix stays ahead",
			message: "DRAFT: fix(a): x",
			want:    "DRAFT: fix(a): [wip] x",
		},
		{
			name:    "only line one is marked",
			message: ":bug: fix(a): x\n\nwhy: because",
			want:    ":bug: fix(a): [wip] x\n\nwhy: because",
		},
	}

	runMark(t, testCases)
}

func TestMarkNormalForm(t *testing.T) {
	t.Parallel()

	testCases := []markCase{
		{
			name:    "a doubled marker is one",
			message: ":bug: fix(b): [wip] [wip] Two",
			want:    ":bug: fix(b): [wip] Two",
		},
		{
			name:    "a marker ahead of the type moves",
			message: ":bug: [wip] fix(b): Old",
			want:    ":bug: fix(b): [wip] Old",
		},
		{
			name:    "a marker ahead of the emoji moves",
			message: "[wip] :bug: fix(b): Lead",
			want:    ":bug: fix(b): [wip] Lead",
		},
		{
			name:    "a spaced scope",
			message: "fix(my scope): Space",
			want:    "fix(my scope): [wip] Space",
		},
		{
			name:    "a marker closing line one is the one git shows",
			message: ":bug: fix(b): [wip]\nHand",
			want:    ":bug: fix(b): [wip]\nHand",
		},
		{
			name:    "folded lines are marked as git shows them",
			message: ":bug: fix(b): One\ntwo\n\nwhy",
			want:    ":bug: fix(b): [wip] One two\n\nwhy",
		},
	}

	runMark(t, testCases)
}

func TestMarkLeavesOneMarkerInTheGitSubject(t *testing.T) {
	t.Parallel()

	testCases := []struct{ name, message string }{
		{name: "closing line one", message: ":bug: fix(b): [wip]\nHand"},
		{name: "plain", message: ":bug: fix(b): Hand"},
		{name: "doubled", message: ":bug: fix(b): [wip] [wip] Two"},
		{name: "ahead of the type", message: ":bug: [wip] fix(b): Old"},
		{name: "ahead of the emoji", message: "[wip] :bug: fix(b): Lead"},
		{name: "folded and doubled", message: ":bug: fix(b): [wip]\n[wip] Hand\n\nbody"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := gitSubject(Mark(testCase.message))
			if count := strings.Count(got, "[wip]"); count != 1 ||
				!strings.HasPrefix(got, ":bug: fix(b): [wip] ") {
				t.Fatalf("git shows %q for %q, want one marker in the slot", got, testCase.message)
			}
		})
	}
}

func TestUnmark(t *testing.T) {
	t.Parallel()

	testCases := []struct{ name, message, want string }{
		{name: "in place", message: ":bug: fix(b): [wip] Hand", want: ":bug: fix(b): Hand"},
		{name: "none", message: ":bug: fix(b): Hand", want: ":bug: fix(b): Hand"},
		{name: "doubled", message: ":bug: fix(b): [wip] [wip] Two", want: ":bug: fix(b): Two"},
		{name: "ahead of the type", message: ":bug: [wip] fix(b): Old", want: ":bug: fix(b): Old"},
		{
			name:    "ahead of the emoji",
			message: "[wip] :bug: fix(b): Lead",
			want:    ":bug: fix(b): Lead",
		},
		{
			name:    "closing line one",
			message: ":bug: fix(b): [wip]\nHand",
			want:    ":bug: fix(b): Hand",
		},
		{
			name:    "a body is kept",
			message: ":bug: fix(b): [wip] Hand\n\nwhy",
			want:    ":bug: fix(b): Hand\n\nwhy",
		},
		{name: "lines without a marker are kept", message: "a\nb", want: "a\nb"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Unmark(testCase.message); got != testCase.want {
				t.Fatalf("Unmark(%q) = %q, want %q", testCase.message, got, testCase.want)
			}
		})
	}
}

func TestGitSubject(t *testing.T) {
	t.Parallel()

	testCases := []struct{ name, message, want string }{
		{name: "single", message: "a", want: "a"},
		{name: "multi", message: "a\nb", want: "a b"},
		{name: "up to the blank line", message: "a\nb\n\nc", want: "a b"},
		{name: "a blank line of spaces ends it", message: "a\n  \nc", want: "a"},
		{name: "trailing spaces go", message: "a  \nb", want: "a b"},
		{name: "empty", message: "", want: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := gitSubject(testCase.message); got != testCase.want {
				t.Fatalf("gitSubject(%q) = %q, want %q", testCase.message, got, testCase.want)
			}
		})
	}
}
