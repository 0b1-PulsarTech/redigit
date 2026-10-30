//go:build integration

package gitstore

import (
	"strings"
	"testing"
)

func TestResolveCommit(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)
	commitTestFiles(t, dir, "seed", map[string][]byte{"a.go": []byte("package a\n")})
	runTestGit(t, dir, "tag", "-a", "-m", "release", "v1")

	head, err := revParse(t.Context(), dir, "HEAD")
	if err != nil {
		t.Fatalf("resolve HEAD: %v", err)
	}

	testCases := []struct {
		name    string
		repo    string
		rev     string
		want    string
		wantErr bool
	}{
		{"a branch name", dir, "main", head, false},
		{"an annotated tag peels to its commit", dir, "v1", head, false},
		{"the full sha", dir, head, head, false},
		{"an unknown rev", dir, "no-such-rev", "", true},
		{"an empty repository", newTestRepo(t), "HEAD", "", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := ResolveCommit(t.Context(), testCase.repo, testCase.rev)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("ResolveCommit() err = %v, wantErr %v", err, testCase.wantErr)
			}

			if got != testCase.want {
				t.Fatalf("ResolveCommit() = %q, want %q", got, testCase.want)
			}

			if err == nil && len(strings.TrimSpace(got)) != len(head) {
				t.Fatalf("ResolveCommit() = %q, want a full object id", got)
			}
		})
	}
}
