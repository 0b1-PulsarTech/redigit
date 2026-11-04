package subjecting

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestPrefixLengthAgreesWithTheMarkerSlot(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		line        string
		isFormatted bool
		want        string
	}{
		{
			name: "full shape", line: ":bug: fix(a): Hand", isFormatted: true,
			want: ":bug: fix(a): [wip] Hand",
		},
		{
			name: "no scope", line: ":gem: feat: Hand", isFormatted: true,
			want: ":gem: feat: [wip] Hand",
		},
		{
			name: "spaced scope", line: ":bug: fix(my scope): Space", isFormatted: true,
			want: ":bug: fix(my scope): [wip] Space",
		},
		{
			name: "slashed scope", line: ":ok_2+x-1: chore(a/b): Hand", isFormatted: true,
			want: ":ok_2+x-1: chore(a/b): [wip] Hand",
		},
		{
			name: "colon in the description", line: ":bug: fix(a): Hand: more", isFormatted: true,
			want: ":bug: fix(a): [wip] Hand: more",
		},
		{name: "empty scope", line: ":bug: fix(): Hand", want: ":bug: [wip] fix(): Hand"},
		{name: "unclosed scope", line: ":bug: fix(a: Hand", want: ":bug: [wip] fix(a: Hand"},
		{name: "nested scope", line: ":bug: fix(a(b)): Hand", want: ":bug: [wip] fix(a(b)): Hand"},
		{name: "capital type", line: ":bug: Fix(a): Hand", want: ":bug: [wip] Fix(a): Hand"},
		{name: "empty type", line: ":bug: (a): Hand", want: ":bug: [wip] (a): Hand"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			prefix, isFormatted := prefixLength(testCase.line)
			if isFormatted != testCase.isFormatted {
				t.Fatalf("prefixLength(%q) = %v", testCase.line, isFormatted)
			}

			if got := entities.MarkWIP(testCase.line); got != testCase.want {
				t.Fatalf("MarkWIP(%q) = %q, want %q", testCase.line, got, testCase.want)
			}

			if isFormatted &&
				testCase.want != testCase.line[:prefix]+"[wip] "+testCase.line[prefix:] {
				t.Fatalf("slot of %q is not the end of the format prefix %d", testCase.line, prefix)
			}
		})
	}
}

func TestPrefixLength(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		line   string
		want   int
		wantOK bool
	}{
		{name: "full shape", line: ":bug: fix(a): Hand", want: len(":bug: fix(a): "), wantOK: true},
		{name: "no scope", line: ":gem: feat: Hand", want: len(":gem: feat: "), wantOK: true},
		{
			name: "emoji with digits and plus", line: ":ok_2+x-1: chore(a/b): Hand",
			want: len(":ok_2+x-1: chore(a/b): "), wantOK: true,
		},
		{name: "no emoji", line: "fix(a): Hand"},
		{name: "empty emoji", line: ":: fix(a): Hand"},
		{name: "emoji with a capital", line: ":Bug: fix(a): Hand"},
		{name: "no type separator", line: ":bug: fix(a) Hand"},
		{name: "empty description", line: ":bug: fix(a): "},
		{name: "empty scope", line: ":bug: fix(): Hand"},
		{name: "unclosed scope", line: ":bug: fix(a: Hand"},
		{name: "nested scope", line: ":bug: fix(a(b)): Hand"},
		{name: "empty type", line: ":bug: (a): Hand"},
		{name: "capital type", line: ":bug: Fix(a): Hand"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, isOK := prefixLength(testCase.line)
			if isOK != testCase.wantOK || got != testCase.want {
				t.Fatalf(
					"prefixLength() = %d, %v, want %d, %v",
					got,
					isOK,
					testCase.want,
					testCase.wantOK,
				)
			}
		})
	}
}
