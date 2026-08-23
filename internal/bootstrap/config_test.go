package bootstrap

import (
	"testing"
	"time"
)

// TestDefaults pins the values a run starts from before flags narrow it. The
// tool that shipped without them is why this file exists: ordering.Config had
// no defaults, so RoundLimit arrived as 0 and the fixed-point loop never ran,
// handing back an ordering that was nothing but its own seed while looking
// settled.
func TestDefaults(t *testing.T) {
	t.Parallel()

	got := Defaults()

	testCases := []struct {
		name string
		got  any
		want any
	}{
		{"rev defaults to HEAD", got.Rev, "HEAD"},
		{"the server command is gopls", got.ServerCommand, "gopls"},
		{"the author name is redigit", got.Author.Name, "redigit"},
		{"the author email is redigit@localhost", got.Author.Email, "redigit@localhost"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.got != testCase.want {
				t.Fatalf("got %v, want %v", testCase.got, testCase.want)
			}
		})
	}

	if want := []string{"-mode=stdio"}; len(got.ServerArgs) != 1 || got.ServerArgs[0] != want[0] {
		t.Fatalf("ServerArgs = %v, want %v", got.ServerArgs, want)
	}

	if !got.Author.Start.Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("Author.Start = %v, want the unix epoch", got.Author.Start)
	}

	// Repo and Rev.BuildTags are left zero on purpose: a default repository
	// path would silently point a run at the wrong tree.
	if got.Repo != "" {
		t.Fatalf("Repo = %q, want empty so --repo is required", got.Repo)
	}

	if got.BuildTags != nil {
		t.Fatalf("BuildTags = %v, want nil", got.BuildTags)
	}
}
