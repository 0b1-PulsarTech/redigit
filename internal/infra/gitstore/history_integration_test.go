//go:build integration

package gitstore

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

// TestHistoryRebuildsGrowingFiles is the end-to-end proof of the write half:
// a file that grows across two commits, an executable that keeps its bit, and a
// final tree that matches the target exactly.
func TestHistoryRebuildsGrowingFiles(t *testing.T) {
	dir := newTestRepo(t)
	commitTestFiles(t, dir, "target", map[string][]byte{
		"a.go":   []byte("One\nTwo\n"),
		"run.sh": []byte("#!/bin/sh\n"),
	})
	runTestGit(t, dir, "update-index", "--chmod=+x", "run.sh")
	runTestGit(t, dir, "commit", "-q", "-m", "make it executable")

	ctx := t.Context()
	author := Identity{
		Name:  "Tester",
		Email: "tester@example.com",
		When:  time.Unix(1000000000, 0).UTC(),
	}
	history := NewHistory(dir, "HEAD", author)

	ref, err := history.Write(ctx, "refs/heads/rebuilt", []rewriting.Commit{
		{Subject: "feat: One", Writes: []rewriting.FileContent{
			{Path: "a.go", Content: []byte("One\n")},
		}},
		{Subject: "feat: Two", Writes: []rewriting.FileContent{
			{Path: "a.go", Content: []byte("One\nTwo\n")},
			{Path: "run.sh", Content: []byte("#!/bin/sh\n")},
		}},
	})
	if err != nil {
		t.Fatalf("Write(): %v", err)
	}

	if err = history.Verify(ctx, ref, "HEAD"); err != nil {
		t.Fatalf("final-tree invariant: %v", err)
	}
}

// TestHistoryIsReproducible pins that the same plan written twice produces the
// same identifier. Without it a comparison between two rebuild rounds compares
// noise, which is exactly the mistake of comparing commit hashes instead of
// trees, one level up.
func TestHistoryIsReproducible(t *testing.T) {
	dir := newTestRepo(t)
	commitTestFiles(t, dir, "target", map[string][]byte{"a.go": []byte("One\n")})

	ctx := t.Context()
	author := Identity{
		Name:  "Tester",
		Email: "tester@example.com",
		When:  time.Unix(1000000000, 0).UTC(),
	}

	commits := []rewriting.Commit{{
		Subject: "feat: One",
		Writes:  []rewriting.FileContent{{Path: "a.go", Content: []byte("One\n")}},
	}}

	first, err := NewHistory(dir, "HEAD", author).Write(ctx, "refs/heads/one", commits)
	if err != nil {
		t.Fatalf("first Write(): %v", err)
	}

	second, err := NewHistory(dir, "HEAD", author).Write(ctx, "refs/heads/two", commits)
	if err != nil {
		t.Fatalf("second Write(): %v", err)
	}

	if first != second {
		t.Fatalf("rebuild is not reproducible: %s then %s", first, second)
	}
}

type replayCase struct {
	mode      string
	content   string
	isBlobDir bool
}

func replayOnce(t *testing.T, testCase replayCase) (dir, sha string) {
	t.Helper()

	dir = newTestRepo(t)
	history := NewHistory(dir, "HEAD", Identity{Name: "Proof", Email: "proof@localhost"})
	scratch := t.TempDir()
	handle := "blob-1"

	if err := os.WriteFile(
		filepath.Join(scratch, handle),
		[]byte(testCase.content),
		0o600,
	); err != nil {
		t.Fatalf("write blob: %v", err)
	}

	if testCase.isBlobDir {
		history = history.WithBlobDir(scratch)
	} else {
		out, err := runGit(t.Context(), dir, "hash-object", "-w", filepath.Join(scratch, handle))
		if err != nil {
			t.Fatalf("hash-object: %v", err)
		}

		handle = strings.TrimSpace(string(out))
	}

	staged, err := history.Replay(t.Context(), "refs/heads/out", []rewriting.Replayed{
		{
			Position: 0, Message: "first\n", At: rewriting.Stamp{Unix: 1700000000, Zone: "+0200"},
			Blobs: []rewriting.BlobRef{{Path: "dir/f", Mode: testCase.mode, Handle: handle}},
		},
		{Position: 1, Message: "empty row\n", At: rewriting.Stamp{Unix: 1700000001, Zone: "-0300"}},
	})
	if err != nil {
		t.Fatalf("Replay() err = %v", err)
	}

	if err = history.Publish(t.Context(), "refs/heads/out", staged); err != nil {
		t.Fatalf("Publish() err = %v", err)
	}

	return dir, staged.Tip
}

func replayInputs() []rewriting.Replayed {
	return []rewriting.Replayed{
		{Position: 0, Message: "first\n", At: rewriting.Stamp{Unix: 1700000000, Zone: "+0200"}},
	}
}

func refNames(t *testing.T, dir string) []string {
	t.Helper()

	out, err := runGit(t.Context(), dir, "for-each-ref", "--format=%(refname)")
	if err != nil {
		t.Fatalf("for-each-ref: %v", err)
	}

	return strings.Fields(string(out))
}

func isScratch(ref string) bool { return strings.HasPrefix(ref, "refs/redigit/") }

func refValue(t *testing.T, dir, ref string) string {
	t.Helper()

	out, err := runGit(t.Context(), dir, "for-each-ref", "--format=%(objectname)", ref)
	if err != nil {
		t.Fatalf("for-each-ref %s: %v", ref, err)
	}

	return strings.TrimSpace(string(out))
}

func TestHistoryReplayStagesBeforeTheRefMoves(t *testing.T) {
	t.Parallel()

	const target = "refs/heads/out"

	testCases := []struct {
		name       string
		isExisting bool
	}{
		{"absent ref stays absent until publish", false},
		{"existing ref keeps its tip until publish", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := newTestRepo(t)
			ctx := t.Context()
			history := NewHistory(dir, "", Identity{Name: "Proof", Email: "proof@localhost"})

			if testCase.isExisting {
				commitTestFiles(t, dir, "seed", map[string][]byte{"a.go": []byte("One\n")})
				runTestGit(t, dir, "update-ref", target, "HEAD")
			}

			wantTip := refValue(t, dir, target)

			staged, err := history.Replay(ctx, target, replayInputs())
			if err != nil {
				t.Fatalf("Replay() err = %v", err)
			}

			if got := refValue(t, dir, target); got != wantTip {
				t.Fatalf("target after Replay = %q, want %q", got, wantTip)
			}

			if got := refValue(t, dir, staged.Scratch); got != staged.Tip {
				t.Fatalf("scratch = %q, want the staged tip %q", got, staged.Tip)
			}

			if err = history.Publish(ctx, target, staged); err != nil {
				t.Fatalf("Publish() err = %v", err)
			}

			if got := refValue(t, dir, target); got != staged.Tip {
				t.Fatalf("target after Publish = %q, want %q", got, staged.Tip)
			}

			if err = history.Discard(ctx, staged); err != nil {
				t.Fatalf("Discard() err = %v", err)
			}

			if got := refNames(t, dir); slices.ContainsFunc(got, isScratch) {
				t.Fatalf("refs after Discard = %v, want the scratch gone", got)
			}
		})
	}
}

func TestHistoryReplayLeavesNoRefOnFailure(t *testing.T) {
	t.Parallel()

	missing := strings.Repeat("0123456789", 4)

	testCases := []struct {
		name    string
		isEarly bool
		commits []rewriting.Replayed
	}{
		{"fast-import refuses a missing object", false, []rewriting.Replayed{{
			Message: "x\n", At: rewriting.Stamp{Unix: 1700000000, Zone: "+0200"},
			Blobs: []rewriting.BlobRef{{Path: "f", Mode: "100644", Handle: missing}},
		}}},
		{"a cancelled context", true, replayInputs()},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := newTestRepo(t)
			history := NewHistory(dir, "", Identity{Name: "Proof", Email: "proof@localhost"})
			ctx, cancel := context.WithCancel(t.Context())

			if testCase.isEarly {
				cancel()
			}

			defer cancel()

			if _, err := history.Replay(ctx, "refs/heads/out", testCase.commits); err == nil {
				t.Fatal("Replay() err = nil, want a failure")
			}

			if got := refNames(t, dir); len(got) != 0 {
				t.Fatalf("refs after a failed Replay = %v, want none", got)
			}
		})
	}
}

func TestHistoryReplay(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		replayCase

		name string
	}{
		{replayCase{"100644", "package a\n", false}, "oid handle"},
		{replayCase{"100644", "package a\n", true}, "blob dir handle"},
		{replayCase{"100755", "#!/bin/sh\n", true}, "executable keeps its bit"},
		{replayCase{"120000", "target.txt", false}, "symlink keeps its mode"},
	}

	const wantLog = "Proof <proof@localhost> 1700000001 -0300|Proof <proof@localhost>\n" +
		"Proof <proof@localhost> 1700000000 +0200|Proof <proof@localhost>\n"

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir, first := replayOnce(t, testCase.replayCase)
			_, second := replayOnce(t, testCase.replayCase)

			if first != second {
				t.Fatalf("Replay() is not deterministic: %s != %s", first, second)
			}

			tree, err := runGit(t.Context(), dir, "ls-tree", "-r", first)
			if err != nil || !strings.HasPrefix(string(tree), testCase.mode+" blob ") {
				t.Fatalf("ls-tree = %q, %v, want mode %s kept", tree, err, testCase.mode)
			}

			log, err := runGit(t.Context(), dir, "log", "--date=format:%z",
				"--format=%an <%ae> %at %ad|%cn <%ce>", first)
			if err != nil || string(log) != wantLog {
				t.Fatalf("log = %q, %v, want %q", log, err, wantLog)
			}

			dates, err := runGit(t.Context(), dir, "log", "--format=%at %aI|%ct %cI", first)
			if err != nil || len(dates) == 0 {
				t.Fatalf("log dates = %q, %v", dates, err)
			}

			for line := range strings.SplitSeq(strings.TrimSpace(string(dates)), "\n") {
				authored, committed, _ := strings.Cut(line, "|")
				if authored != committed {
					t.Fatalf("author date %q != committer date %q", authored, committed)
				}
			}

			commits, err := runGit(t.Context(), dir, "rev-list", "--count", first)
			if err != nil || strings.TrimSpace(string(commits)) != "2" {
				t.Fatalf("rev-list = %q, %v, want 2 commits", commits, err)
			}
		})
	}
}
