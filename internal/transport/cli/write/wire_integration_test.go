//go:build integration

package write

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
)

type fixture struct {
	repo        string
	oidManifest string
	dirManifest string
	blobDir     string
	config      string
}

func gitOut(t *testing.T, repo string, args ...string) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", append([]string{"-C", repo}, args...)...).
		CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newFixture(t *testing.T, subject string) fixture {
	t.Helper()

	scratch := t.TempDir()
	fix := fixture{
		repo:        filepath.Join(scratch, "repo"),
		oidManifest: filepath.Join(scratch, "oid.jsonl"),
		dirManifest: filepath.Join(scratch, "dir.jsonl"),
		blobDir:     filepath.Join(scratch, "blobs"),
		config:      filepath.Join(scratch, "cfg.json"),
	}

	writeFile(t, filepath.Join(fix.repo, ".keep"), "")
	gitOut(t, fix.repo, "init", "-q", "-b", "main")
	gitOut(t, fix.repo, "config", "user.email", "t@example.com")
	gitOut(t, fix.repo, "config", "user.name", "T")

	names := []string{"a.go", "b.go"}
	oidLines, dirLines := make([]string, 0, len(names)), make([]string, 0, len(names))

	for position, name := range names {
		writeFile(t, filepath.Join(fix.repo, name), "package "+name[:1]+"\n")
		writeFile(t, filepath.Join(fix.blobDir, "h-"+name), "package "+name[:1]+"\n")

		oid := gitOut(t, fix.repo, "hash-object", "-w", name)
		date := fmt.Sprintf("%d +0%d00", 1700000000+position*1000, position+1)

		gitOut(t, fix.repo, "add", name)
		gitOut(t, fix.repo, "commit", "-q", "--date", date, "-m", name)

		row := `{"position":%d,"subject":%q,"blobs":[{"path":%q,"mode":"100644","handle":%q}]}`
		oidLines = append(oidLines, fmt.Sprintf(row, position, subject, name, oid))
		dirLines = append(dirLines, fmt.Sprintf(row, position, subject, name, "h-"+name))
	}

	writeFile(t, fix.oidManifest, strings.Join(oidLines, "\n")+"\n")
	writeFile(t, fix.dirManifest, strings.Join(dirLines, "\n")+"\n")
	writeFile(t, fix.config, `{"write":{"identity":{"name":"P","email":"p@x"},"dateRef":"HEAD"}}`)

	return fix
}

func runWrite(t *testing.T, fix fixture, ref string, extra ...string) (result, error) {
	t.Helper()

	args := append([]string{"--repo", fix.repo, "--ref", ref, "--config", fix.config}, extra...)

	var stdout bytes.Buffer
	if err := Run(t.Context(), args, &stdout); err != nil {
		return result{}, err
	}

	var got result
	if err := json.Unmarshal(
		stdout.Bytes(),
		&got,
	); err != nil || got.Ref != ref ||
		got.Commits != 2 {
		t.Fatalf("Run() printed %q (%v), want ref %s over 2 commits", stdout.String(), err, ref)
	}

	return got, nil
}

func TestRunReplaysDeterministically(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		subject string
		extra   []string
		wantErr error
	}{
		{"verified against the target", "feat: x", []string{"--verify", "HEAD"}, nil},
		{"verify mismatch", "feat: x", []string{"--verify", "HEAD~1"}, apperr.ErrInvariant},
		{"draft refused", "DRAFT: x", nil, apperr.ErrIncomplete},
		{"draft allowed", "DRAFT: x", []string{"--allow-draft"}, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := newFixture(t, testCase.subject)
			args := append([]string{"--manifest", fix.oidManifest}, testCase.extra...)

			first, err := runWrite(t, fix, "refs/heads/w1", args...)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Run() err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr != nil {
				return
			}

			second, err := runWrite(t, fix, "refs/heads/w2", args...)
			if err != nil || first.Commit != second.Commit {
				t.Fatalf("second Run() = %s, %v, want %s", second.Commit, err, first.Commit)
			}

			if left := gitOut(t, fix.repo, "for-each-ref", "refs/redigit"); left != "" {
				t.Fatalf("scratch refs left behind: %q", left)
			}
		})
	}
}

func TestRunBlobDirMatchesOidHandles(t *testing.T) {
	t.Parallel()

	fix := newFixture(t, "feat: x")

	byOid, err := runWrite(t, fix, "refs/heads/oid", "--manifest", fix.oidManifest)
	if err != nil {
		t.Fatalf("oid Run() err = %v", err)
	}

	byDir, err := runWrite(
		t,
		fix,
		"refs/heads/dir",
		"--manifest",
		fix.dirManifest,
		"--blob-dir",
		fix.blobDir,
	)
	if err != nil || byDir.Commit != byOid.Commit {
		t.Fatalf("blob-dir Run() = %s, %v, want %s", byDir.Commit, err, byOid.Commit)
	}
}

func TestRunVerifyFailureLeavesTheRefAlone(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		isByDir    bool
		isExisting bool
	}{
		{"oid handles, absent ref", false, false},
		{"blob dir handles, absent ref", true, false},
		{"oid handles, existing ref", false, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := newFixture(t, "feat: x")
			args := []string{"--manifest", fix.oidManifest, "--verify", "HEAD~1"}

			if testCase.isByDir {
				args = []string{
					"--manifest", fix.dirManifest, "--blob-dir", fix.blobDir, "--verify", "HEAD~1",
				}
			}

			if testCase.isExisting {
				gitOut(t, fix.repo, "update-ref", "refs/heads/w1", "HEAD~1")
			}

			before := gitOut(t, fix.repo, "for-each-ref", "refs/heads/w1")

			_, err := runWrite(t, fix, "refs/heads/w1", args...)
			if !errors.Is(err, apperr.ErrInvariant) {
				t.Fatalf("Run() err = %v, want the invariant error", err)
			}

			after := gitOut(t, fix.repo, "for-each-ref", "refs/heads/w1")
			if after != before {
				t.Fatalf("refs/heads/w1 = %q, want it left at %q", after, before)
			}

			if left := gitOut(t, fix.repo, "for-each-ref", "refs/redigit"); left != "" {
				t.Fatalf("scratch refs left behind: %q", left)
			}
		})
	}
}

func TestRunMissingOidSurfacesFastImportStderr(t *testing.T) {
	t.Parallel()

	const missingOid = "0123456789abcdef0123456789abcdef01234567"

	fix := newFixture(t, "feat: x")
	row := `{"position":0,"subject":"feat: x","blobs":[{"path":"a.go","mode":"100644","handle":%q}]}`
	manifest := filepath.Join(filepath.Dir(fix.repo), "missing.jsonl")
	writeFile(t, manifest, fmt.Sprintf(row, missingOid)+"\n")

	_, err := runWrite(t, fix, "refs/heads/w1", "--manifest", manifest)
	if err == nil || !strings.Contains(err.Error(), missingOid) {
		t.Fatalf("Run() err = %v, want fast-import stderr naming %s", err, missingOid)
	}
}
