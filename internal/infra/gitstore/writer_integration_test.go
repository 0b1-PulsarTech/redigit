//go:build integration

package gitstore

import (
	"testing"
)

// twoCommitFixture is a small synthetic history: a first commit that writes
// two paths, and a second that adds one path and deletes another.
func twoCommitFixture() []WriteCommit {
	return []WriteCommit{
		{
			Author:    identityAt(1700000000),
			Committer: identityAt(1700000001),
			Message:   "first",
			Writes: []FileWrite{
				{Path: "a.go", Content: []byte("package a\n")},
				{Path: "sub dir/b.go", Content: []byte("package b\n")},
			},
		},
		{
			Author:    identityAt(1700000010),
			Committer: identityAt(1700000011),
			Message:   "second",
			Writes: []FileWrite{
				{Path: "c.go", Content: []byte("package c\n")},
			},
			Deletes: []string{"sub dir/b.go"},
		},
	}
}

func TestWriterWriteMatchesIndependentTarget(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)

	if _, err := NewWriter(
		dir,
	).Write(t.Context(), "refs/heads/out", twoCommitFixture()); err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	// Build the target tree independently of the writer: a plain commit
	// holding exactly the final state (a.go and c.go; sub dir/b.go absent).
	commitTestFiles(t, dir, "target", map[string][]byte{
		"a.go": []byte("package a\n"),
		"c.go": []byte("package c\n"),
	})
	runTestGit(t, dir, "update-ref", "refs/heads/target", "main")

	if err := Check(t.Context(), dir, "refs/heads/out", "refs/heads/target"); err != nil {
		t.Fatalf("Check() err = %v, want the written tree to match the target", err)
	}
}

func TestWriterWriteIsDeterministic(t *testing.T) {
	t.Parallel()

	shaOf := func(dir string) string {
		sha, err := NewWriter(dir).Write(t.Context(), "refs/heads/out", twoCommitFixture())
		if err != nil {
			t.Fatalf("Write() err = %v", err)
		}

		return sha
	}

	sha1 := shaOf(newTestRepo(t))
	sha2 := shaOf(newTestRepo(t))

	if sha1 != sha2 {
		t.Fatalf("Write() is not deterministic: %s != %s", sha1, sha2)
	}
}

func TestWriterWriteDeleteIsGenuinelyAbsent(t *testing.T) {
	t.Parallel()

	dir := newTestRepo(t)

	if _, err := NewWriter(
		dir,
	).Write(t.Context(), "refs/heads/out", twoCommitFixture()); err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	r := NewReader(dir, "refs/heads/out")

	files, err := r.finalFiles(t.Context())
	if err != nil {
		t.Fatalf("finalFiles() err = %v", err)
	}

	for _, path := range files {
		if path == "sub dir/b.go" {
			t.Fatalf("finalFiles() still lists the deleted path, want it absent")
		}
	}

	if _, err := r.Read(t.Context(), "sub dir/b.go"); err == nil {
		t.Fatalf("Read() of the deleted path succeeded, want an error")
	}
}
