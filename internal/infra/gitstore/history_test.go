package gitstore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

const (
	freshDir   = "<fresh dir>"
	missingDir = "<missing dir>"
)

func replayWith(
	t *testing.T,
	blobDir, zone string,
	blobs []rewriting.BlobRef,
) error {
	t.Helper()

	author := Identity{Name: "Proof", Email: "proof@localhost"}
	history := NewHistory(t.TempDir(), "HEAD", author).WithBlobDir(blobDir)

	_, err := history.Replay(t.Context(), "refs/heads/out", []rewriting.Replayed{{
		At:    rewriting.Stamp{Unix: 1, Zone: zone},
		Blobs: blobs,
	}})

	return err
}

func TestHistoryReplayRejects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		blobDir    string
		handle     string
		wantErr    error
		hasNoBlobs bool
	}{
		{name: "oid handle of 43 dashes", handle: strings.Repeat("-", 43), wantErr: ErrBadHandle},
		{name: "oid handle that is too short", handle: "abcdef", wantErr: ErrBadHandle},
		{name: "oid handle of 39", handle: strings.Repeat("a", 39), wantErr: ErrBadHandle},
		{name: "oid handle of 41", handle: strings.Repeat("a", 41), wantErr: ErrBadHandle},
		{name: "oid handle of 42", handle: strings.Repeat("a", 42), wantErr: ErrBadHandle},
		{name: "oid handle of 100", handle: strings.Repeat("a", 100), wantErr: ErrBadHandle},
		{name: "oid handle in upper case", handle: strings.Repeat("A", 40), wantErr: ErrBadHandle},
		{name: "oid handle of 40 non-hex", handle: strings.Repeat("g", 40), wantErr: ErrBadHandle},
		{
			name: "blob dir handle escaping the dir", blobDir: freshDir,
			handle: "../etc/passwd", wantErr: ErrBadHandle,
		},
		{
			name: "blob dir handle with no file", blobDir: freshDir, handle: "missing",
			wantErr: fs.ErrNotExist,
		},
		{
			name: "missing file named like the escape text", blobDir: freshDir,
			handle: "path escapes from parent", wantErr: fs.ErrNotExist,
		},
		{
			name: "blob dir that does not exist", blobDir: missingDir, handle: "missing",
			wantErr: fs.ErrNotExist,
		},
		{
			name: "blob dir that does not exist and no blobs", blobDir: missingDir,
			wantErr: fs.ErrNotExist, hasNoBlobs: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			blobDir := testCase.blobDir
			switch blobDir {
			case freshDir:
				blobDir = t.TempDir()
			case missingDir:
				blobDir = filepath.Join(t.TempDir(), "absent")
			}

			var blobs []rewriting.BlobRef
			if !testCase.hasNoBlobs {
				blobs = []rewriting.BlobRef{{Path: "a", Mode: "100644", Handle: testCase.handle}}
			}

			if err := replayWith(t, blobDir, "+0000", blobs); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Replay() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestHistoryReplayNamesTheFailingCommit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		commit  rewriting.Replayed
		wantAll []string
	}{
		{
			"bad blob handle",
			rewriting.Replayed{
				Position: 7, At: rewriting.Stamp{Unix: 1, Zone: "+0000"},
				Blobs: []rewriting.BlobRef{{Path: "dir/x.go", Mode: "100644", Handle: "abcdef"}},
			},
			[]string{"position 7", `"dir/x.go"`},
		},
		{
			"bad zone",
			rewriting.Replayed{Position: 9, At: rewriting.Stamp{Unix: 1, Zone: "UTC"}},
			[]string{"position 9"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			author := Identity{Name: "Proof", Email: "proof@localhost"}
			history := NewHistory(t.TempDir(), "HEAD", author)

			_, err := history.Replay(
				t.Context(), "refs/heads/out", []rewriting.Replayed{testCase.commit},
			)
			if err == nil {
				t.Fatalf("Replay() err = nil, want a refusal")
			}

			for _, want := range testCase.wantAll {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("Replay() err = %q, want it to name %s", err, want)
				}
			}
		})
	}
}

func TestHistoryReplayRejectsBadZone(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		zone string
	}{
		{"zone that is not an offset", "UTC"},
		{"empty zone", ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := replayWith(t, "", testCase.zone, nil)
			if _, isParse := errors.AsType[*time.ParseError](err); !isParse {
				t.Fatalf("Replay() err = %v, want a *time.ParseError", err)
			}
		})
	}
}

func TestHistoryReplayBlobDirStaysInside(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		handle string
		link   func(t *testing.T, dir, outside string)
	}{
		{
			"handle that is a symlink to a file outside",
			"link",
			func(t *testing.T, dir, outside string) {
				t.Helper()

				target := filepath.Join(outside, "secret")
				if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			},
		},
		{
			"handle under a directory symlinked outside",
			"sub/secret",
			func(t *testing.T, dir, outside string) {
				t.Helper()

				if err := os.Symlink(outside, filepath.Join(dir, "sub")); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir, outside := t.TempDir(), t.TempDir()
			secret := filepath.Join(outside, "secret")

			if err := os.WriteFile(secret, []byte("s"), 0o600); err != nil {
				t.Fatalf("write secret: %v", err)
			}

			testCase.link(t, dir, outside)

			author := Identity{Name: "Proof", Email: "proof@localhost"}
			history := NewHistory(t.TempDir(), "HEAD", author).WithBlobDir(dir)

			_, err := history.Replay(t.Context(), "refs/heads/out", []rewriting.Replayed{{
				At:    rewriting.Stamp{Unix: 1, Zone: "+0000"},
				Blobs: []rewriting.BlobRef{{Path: "a", Mode: "100644", Handle: testCase.handle}},
			}})
			if !errors.Is(err, ErrBadHandle) {
				t.Fatalf("Replay() err = %v, want ErrBadHandle", err)
			}
		})
	}
}
