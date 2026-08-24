package entities

import "testing"

func TestMarkWIP(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		subject string
		want    string
	}{
		{
			// The marker goes after "type(scope): " and before the
			// description, which stays capitalised.
			name:    "a conventional subject",
			subject: "feat(chatbrain): Add the reply executor",
			want:    "feat(chatbrain): [wip] Add the reply executor",
		},
		{
			// A draft prefix stays ahead of both.
			name:    "a drafted subject",
			subject: DraftMarker + "chore(shared): SoftDelete and 1 more",
			want:    DraftMarker + "chore(shared): [wip] SoftDelete and 1 more",
		},
		{
			name:    "already marked",
			subject: "feat(a): [wip] Already",
			want:    "feat(a): [wip] Already",
		},
		{
			name:    "no conventional prefix at all",
			subject: "Something plain",
			want:    "[wip] Something plain",
		},
		{
			// The description may hold ": " of its own; the marker goes after
			// the FIRST one, which is the type/scope separator.
			name:    "a description holding a colon",
			subject: "fix(a): Handle this: and that",
			want:    "fix(a): [wip] Handle this: and that",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := MarkWIP(testCase.subject)
			if got != testCase.want {
				t.Fatalf("MarkWIP(%q) = %q, want %q", testCase.subject, got, testCase.want)
			}

			if !IsWIPSubject(got) {
				t.Fatalf("IsWIPSubject(%q) = false", got)
			}

			// Marking twice changes nothing.
			if again := MarkWIP(got); again != got {
				t.Fatalf("MarkWIP twice = %q, want %q", again, got)
			}
		})
	}
}
