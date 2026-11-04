package entities

import "testing"

type markCase struct {
	name    string
	subject string
	want    string
}

func runMarkWIP(t *testing.T, testCases []markCase) {
	t.Helper()

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

			if again := MarkWIP(got); again != got {
				t.Fatalf("MarkWIP twice = %q, want %q", again, got)
			}
		})
	}
}

func TestMarkWIPKeepsWords(t *testing.T) {
	t.Parallel()

	testCases := []markCase{
		{
			name:    "a marker explained in the description is text",
			subject: ":gem: feat(b): Keep the [wip] marker in docs",
			want:    ":gem: feat(b): [wip] Keep the [wip] marker in docs",
		},
		{
			name:    "capitals are text",
			subject: ":gem: feat(b): [WIP] One",
			want:    ":gem: feat(b): [wip] [WIP] One",
		},
		{
			name:    "a marker glued to the description is text",
			subject: "feat(b): [wip]One",
			want:    "feat(b): [wip] [wip]One",
		},
	}

	runMarkWIP(t, testCases)
}

func TestMarkWIPNormalForm(t *testing.T) {
	t.Parallel()

	testCases := []markCase{
		{
			name:    "a doubled marker is one",
			subject: ":bug: fix(b): [wip] [wip] Two",
			want:    ":bug: fix(b): [wip] Two",
		},
		{
			name:    "a marker ahead of the type moves",
			subject: ":bug: [wip] fix(b): Old",
			want:    ":bug: fix(b): [wip] Old",
		},
		{
			name:    "a marker ahead of the emoji moves",
			subject: "[wip] :bug: fix(b): Lead",
			want:    ":bug: fix(b): [wip] Lead",
		},
		{
			name:    "a drafted marker ahead of the emoji moves",
			subject: DraftMarker + "[wip] :gem: feat(a): Gen",
			want:    DraftMarker + ":gem: feat(a): [wip] Gen",
		},
		{
			name:    "a drafted marker ahead of the type moves",
			subject: DraftMarker + ":gem: [wip] feat(a): Gen",
			want:    DraftMarker + ":gem: feat(a): [wip] Gen",
		},
		{
			name:    "a scope holding a space",
			subject: "fix(my scope): Space",
			want:    "fix(my scope): [wip] Space",
		},
		{
			name:    "a scope holding a space after an emoji",
			subject: ":bug: fix(my scope): [wip] Space",
			want:    ":bug: fix(my scope): [wip] Space",
		},
		{
			name:    "a scope holding a colon and a space",
			subject: "fix(a: b): Hand",
			want:    "fix(a: b): [wip] Hand",
		},
		{
			name:    "an unclosed scope is no type",
			subject: ":bug: fix(a: Hand",
			want:    ":bug: [wip] fix(a: Hand",
		},
	}

	runMarkWIP(t, testCases)
}

func TestMarkWIP(t *testing.T) {
	t.Parallel()

	testCases := []markCase{
		{
			name:    "a conventional subject",
			subject: "feat(chatbrain): Add the reply executor",
			want:    "feat(chatbrain): [wip] Add the reply executor",
		},
		{
			name:    "a drafted subject",
			subject: DraftMarker + "chore(shared): SoftDelete and 1 more",
			want:    DraftMarker + "chore(shared): [wip] SoftDelete and 1 more",
		},
		{
			name:    "an emoji subject",
			subject: ":bug: fix(b): Hand",
			want:    ":bug: fix(b): [wip] Hand",
		},
		{
			name:    "a drafted emoji subject",
			subject: DraftMarker + ":gem: feat(a): Gen",
			want:    DraftMarker + ":gem: feat(a): [wip] Gen",
		},
		{
			name:    "already marked",
			subject: "feat(a): [wip] Already",
			want:    "feat(a): [wip] Already",
		},
		{
			name:    "already marked after an emoji",
			subject: ":bug: fix(b): [wip] Hand",
			want:    ":bug: fix(b): [wip] Hand",
		},
		{
			name:    "no conventional prefix at all",
			subject: "Something plain",
			want:    "[wip] Something plain",
		},
		{
			name:    "an emoji and no type",
			subject: ":gem: Something plain",
			want:    ":gem: [wip] Something plain",
		},
		{
			name:    "a description holding a colon",
			subject: "fix(a): Handle this: and that",
			want:    "fix(a): [wip] Handle this: and that",
		},
	}

	runMarkWIP(t, testCases)
}

func TestIsWIPSubject(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		subject string
		want    bool
	}{
		{name: "after type and scope", subject: "feat(a): [wip] s", want: true},
		{name: "after an emoji and type", subject: ":bug: fix(b): [wip] s", want: true},
		{name: "after a draft prefix", subject: "DRAFT: feat(a): [wip] s", want: true},
		{
			name:    "after a draft prefix and an emoji",
			subject: "DRAFT: :gem: feat(a): [wip] s",
			want:    true,
		},
		{name: "leading a plain subject", subject: "[wip] s", want: true},
		{name: "leading after an emoji", subject: ":gem: [wip] s", want: true},
		{name: "ahead of a type", subject: ":gem: [wip] feat(a): s", want: true},
		{name: "ahead of an emoji", subject: "[wip] :bug: fix(b): Lead", want: true},
		{name: "doubled", subject: ":bug: fix(b): [wip] [wip] Two", want: true},
		{name: "after a spaced scope", subject: "fix(my scope): [wip] s", want: true},
		{name: "after a scope holding a colon", subject: "fix(a: b): [wip] s", want: true},
		{name: "inside the scope", subject: "fix(a: [wip] b): s", want: false},
		{name: "mid description", subject: "docs(a): explain the [wip] marker", want: false},
		{name: "after a second colon", subject: "feat(a): add x: [wip] y", want: false},
		{name: "capitals", subject: "feat(a): [WIP] s", want: false},
		{name: "glued", subject: "feat(a): [wip]s", want: false},
		{name: "no marker", subject: "feat(a): s", want: false},
		{name: "empty", subject: "", want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := IsWIPSubject(testCase.subject); got != testCase.want {
				t.Fatalf("IsWIPSubject(%q) = %v, want %v", testCase.subject, got, testCase.want)
			}
		})
	}
}

func TestUnmarkWIP(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		subject string
		want    string
	}{
		{name: "in place", subject: ":bug: fix(b): [wip] Hand", want: ":bug: fix(b): Hand"},
		{name: "after a draft", subject: "DRAFT: feat(a): [wip] Gen", want: "DRAFT: feat(a): Gen"},
		{name: "leading", subject: "[wip] plain", want: "plain"},
		{
			name:    "words elsewhere stay",
			subject: ":gem: feat(b): Keep the [wip] marker",
			want:    ":gem: feat(b): Keep the [wip] marker",
		},
		{
			name:    "a doubled marker all goes",
			subject: ":bug: fix(b): [wip] [wip] Two",
			want:    ":bug: fix(b): Two",
		},
		{
			name:    "a marker ahead of the type goes",
			subject: ":bug: [wip] fix(b): Old",
			want:    ":bug: fix(b): Old",
		},
		{
			name:    "a marker ahead of the emoji goes",
			subject: "[wip] :bug: fix(b): Lead",
			want:    ":bug: fix(b): Lead",
		},
		{
			name:    "a drafted marker ahead of the type goes",
			subject: "DRAFT: :gem: [wip] feat(a): Gen",
			want:    "DRAFT: :gem: feat(a): Gen",
		},
		{
			name:    "a spaced scope",
			subject: ":bug: fix(my scope): [wip] Space",
			want:    ":bug: fix(my scope): Space",
		},
		{
			name:    "a scope holding a colon and a space",
			subject: "fix(a: b): [wip] Hand",
			want:    "fix(a: b): Hand",
		},
		{
			name:    "only the marker slot goes",
			subject: "feat(b): [wip] Keep the [wip] marker",
			want:    "feat(b): Keep the [wip] marker",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := UnmarkWIP(testCase.subject); got != testCase.want {
				t.Fatalf("UnmarkWIP(%q) = %q, want %q", testCase.subject, got, testCase.want)
			}
		})
	}
}

func TestIsDraft(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		subject string
		want    bool
	}{
		{"generated", DraftMarker + "feat(lead): Create", true},
		{"authored", ":seedling: feat(lead): Register a lead", false},
		{"empty", "", false},
		{"the marker only counts at the start", "feat: " + DraftMarker, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := IsDraft(testCase.subject); got != testCase.want {
				t.Fatalf("IsDraft(%q) = %v, want %v", testCase.subject, got, testCase.want)
			}
		})
	}
}
