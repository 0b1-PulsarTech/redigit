package gitstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readWrittenFile(dir, path string) ([]byte, error) {
	content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	if err != nil {
		return nil, fmt.Errorf("read written file: %w", err)
	}

	return content, nil
}

func TestBlobStorePutRefusesUnsupportedModeBeforeWriting(t *testing.T) {
	t.Parallel()

	// repoPath names no real repository and the store never starts a
	// process: an invalid mode is caught before anything is written, git or
	// otherwise, so this needs no git binary at all.
	s := NewBlobStore("/does/not/exist")

	_, err := s.Put(t.Context(), "vendor/lib", "160000", []byte("x"))

	var modeErr *UnsupportedModeError
	if !errors.As(err, &modeErr) {
		t.Fatalf("Put() err = %v, want *UnsupportedModeError", err)
	}

	if modeErr.Path != "vendor/lib" {
		t.Fatalf("Path = %q, want %q", modeErr.Path, "vendor/lib")
	}

	if s.cmd != nil {
		t.Fatalf("Put() started a hash-object process before validating the mode")
	}
}

func TestBlobStoreCloseBeforePutIsANoOp(t *testing.T) {
	t.Parallel()

	s := NewBlobStore(t.TempDir())

	if err := s.Close(); err != nil {
		t.Fatalf("Close() err = %v, want nil", err)
	}
}

func TestDirSinkPutRefusesUnsupportedModeBeforeWriting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sink := NewDirSink(dir)

	_, err := sink.Put(t.Context(), "vendor/lib", "160000", []byte("x"))

	var modeErr *UnsupportedModeError
	if !errors.As(err, &modeErr) {
		t.Fatalf("Put() err = %v, want *UnsupportedModeError", err)
	}

	if modeErr.Path != "vendor/lib" {
		t.Fatalf("Path = %q, want %q", modeErr.Path, "vendor/lib")
	}
}

func TestDirSinkPutRefusesPathEscapingDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
	}{
		{"parent traversal", "../outside.go"},
		{"nested parent traversal", "sub/../../outside.go"},
		{"absolute-looking traversal", "../../../../etc/passwd"},
		{"empty path", ""},
		{"current dir", "."},
		{"current dir with trailing slash", "./"},
		{"traversal that lexically cancels to the root", "sub/.."},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			sink := NewDirSink(dir)

			canary := filepath.Join(dir, "canary.txt")
			if err := os.WriteFile(canary, []byte("keep"), 0o600); err != nil {
				t.Fatalf("seed canary file: %v", err)
			}

			seedSubdir := filepath.Join(dir, "seeded")
			if err := os.Mkdir(seedSubdir, 0o750); err != nil {
				t.Fatalf("seed canary subdirectory: %v", err)
			}

			_, err := sink.Put(t.Context(), testCase.path, ModeFile, []byte("x"))

			var escapeErr *PathEscapesDirError
			if !errors.As(err, &escapeErr) {
				t.Fatalf("Put() err = %v, want *PathEscapesDirError", err)
			}

			if escapeErr.Path != testCase.path {
				t.Fatalf("Path = %q, want %q", escapeErr.Path, testCase.path)
			}

			if _, statErr := os.Stat(filepath.Join(dir, "..", "outside.go")); statErr == nil {
				t.Fatalf("Put() wrote outside dir despite refusing the path")
			}

			if _, statErr := os.Stat(dir); statErr != nil {
				t.Fatalf("Put() left the sink directory itself gone: %v", statErr)
			}

			canaryContent, err := os.ReadFile(canary)
			if err != nil {
				t.Fatalf("Put() removed the canary file: %v", err)
			}

			if string(canaryContent) != "keep" {
				t.Fatalf("canary content = %q, want %q", canaryContent, "keep")
			}

			if _, statErr := os.Stat(seedSubdir); statErr != nil {
				t.Fatalf("Put() removed the seeded subdirectory: %v", statErr)
			}
		})
	}
}

func TestDirSinkPutWritesARealSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sink := NewDirSink(dir)

	target := "../elsewhere/target.go"

	oid, err := sink.Put(t.Context(), "link.go", ModeSymlink, []byte(target))
	if err != nil {
		t.Fatalf("Put() err = %v", err)
	}

	if oid != "" {
		t.Fatalf("Put() oid = %q, want empty", oid)
	}

	full := filepath.Join(dir, "link.go")

	info, err := os.Lstat(full)
	if err != nil {
		t.Fatalf("Lstat() err = %v", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("Put() did not create a symlink, mode = %v", info.Mode())
	}

	got, err := os.Readlink(full)
	if err != nil {
		t.Fatalf("Readlink() err = %v", err)
	}

	if got != target {
		t.Fatalf("symlink target = %q, want %q", got, target)
	}
}

// dirSinkOverwriteCase is one path-rewritten-with-a-different-mode fixture:
// firstMode/firstBody land at "path.go" first, then secondMode/secondBody
// overwrite it, and the assertion checks what actually landed matches the
// second write, not a leftover from the first.
type dirSinkOverwriteCase struct {
	name       string
	firstMode  FileMode
	firstBody  []byte
	secondMode FileMode
	secondBody []byte
}

func TestDirSinkPutReplacesWhateverModeStoodAtThePathBefore(t *testing.T) {
	t.Parallel()

	testCases := []dirSinkOverwriteCase{
		{"file to executable", ModeFile, []byte("a"), ModeExecutable, []byte("b")},
		{"executable to file", ModeExecutable, []byte("a"), ModeFile, []byte("b")},
		{
			"symlink to file",
			ModeSymlink,
			[]byte("../old-target"),
			ModeFile,
			[]byte("plain content"),
		},
		{
			"file to symlink",
			ModeFile,
			[]byte("plain content"),
			ModeSymlink,
			[]byte("../new-target"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertDirSinkOverwriteReplacesMode(t, testCase)
		})
	}
}

func assertDirSinkOverwriteReplacesMode(t *testing.T, testCase dirSinkOverwriteCase) {
	t.Helper()

	dir := t.TempDir()
	sink := NewDirSink(dir)
	full := filepath.Join(dir, "path.go")

	if _, err := sink.Put(
		t.Context(),
		"path.go",
		testCase.firstMode,
		testCase.firstBody,
	); err != nil {
		t.Fatalf("first Put() err = %v", err)
	}

	if _, err := sink.Put(
		t.Context(),
		"path.go",
		testCase.secondMode,
		testCase.secondBody,
	); err != nil {
		t.Fatalf("second Put() err = %v", err)
	}

	info, err := os.Lstat(full)
	if err != nil {
		t.Fatalf("Lstat() err = %v", err)
	}

	isSymlink := info.Mode()&os.ModeSymlink != 0
	if wantSymlink := testCase.secondMode == ModeSymlink; isSymlink != wantSymlink {
		t.Fatalf("is symlink = %v, want %v", isSymlink, wantSymlink)
	}

	if testCase.secondMode == ModeSymlink {
		assertDirSinkSymlinkTarget(t, full, string(testCase.secondBody))

		return
	}

	assertDirSinkFileContentAndPerm(t, full, info, testCase.secondMode, testCase.secondBody)
}

func assertDirSinkSymlinkTarget(t *testing.T, full, wantTarget string) {
	t.Helper()

	got, err := os.Readlink(full)
	if err != nil {
		t.Fatalf("Readlink() err = %v", err)
	}

	if got != wantTarget {
		t.Fatalf("symlink target = %q, want %q", got, wantTarget)
	}
}

func assertDirSinkFileContentAndPerm(
	t *testing.T,
	full string,
	info os.FileInfo,
	mode FileMode,
	wantBody []byte,
) {
	t.Helper()

	gotPerm := info.Mode().Perm()

	wantPerm := os.FileMode(dirSinkFilePerm)
	if mode == ModeExecutable {
		wantPerm = dirSinkExecFilePerm
	}

	if gotPerm != wantPerm {
		t.Fatalf("perm = %o, want %o: stale permission bits were kept", gotPerm, wantPerm)
	}

	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("ReadFile() err = %v", err)
	}

	if string(got) != string(wantBody) {
		t.Fatalf("content = %q, want %q", got, wantBody)
	}
}

func TestDirSinkPutWritesUnderDirAndReturnsNoOID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		mode FileMode
	}{
		{"a plain file", "modules/lead/lead.go", ModeFile},
		{"a path with a space", "sub dir/b.go", ModeFile},
		{"a path with a quote", `weird"name.go`, ModeFile},
		{"an executable script", "scripts/run.sh", ModeExecutable},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			sink := NewDirSink(dir)
			content := []byte("package a\n")

			oid, err := sink.Put(t.Context(), testCase.path, testCase.mode, content)
			if err != nil {
				t.Fatalf("Put() err = %v", err)
			}

			if oid != "" {
				t.Fatalf("Put() oid = %q, want empty: a directory sink has no object database", oid)
			}

			got, err := readWrittenFile(dir, testCase.path)
			if err != nil {
				t.Fatalf("read written file: %v", err)
			}

			if string(got) != string(content) {
				t.Fatalf("written content = %q, want %q", got, content)
			}
		})
	}
}

// TestAuditDirSinkStaysInsideItsBase pins the property that matters, rather
// than a list of strings somebody guessed: whatever the sink accepts must land
// strictly under its base, and nothing outside the base may ever disappear.
func TestAuditDirSinkStaysInsideItsBase(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "sink")

	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(root, "canary.txt")
	if err := os.WriteFile(outside, []byte("do not delete"), 0o600); err != nil {
		t.Fatal(err)
	}

	sink := NewDirSink(base)

	for _, path := range []string{
		"", ".", "..", "sub/..", "../canary.txt", "a/../../canary.txt",
		"./", "../", "sub/../..", base, root, "/etc/passwd", "/",
		"a/b/../../..", "....//", "sub/./..", "\\..\\..", "a\x00b",
	} {
		_, err := sink.Put(context.Background(), path, ModeFile, []byte("x"))

		// The canary is the real assertion: refused or accepted, nothing
		// outside the base may be touched.
		if _, statErr := os.Stat(outside); statErr != nil {
			t.Fatalf("path %q destroyed a file outside the sink: %v", path, statErr)
		}

		if _, statErr := os.Stat(base); statErr != nil {
			t.Fatalf("path %q removed the sink directory itself: %v", path, statErr)
		}

		if err != nil {
			continue
		}

		// Accepted: it must have landed strictly inside base.
		written := filepath.Join(filepath.Clean(base), filepath.FromSlash(path))

		if !strings.HasPrefix(written, filepath.Clean(base)+string(filepath.Separator)) {
			t.Errorf("path %q was accepted but resolves to %q, outside the base", path, written)
		}
	}

	if _, err := sink.Put(context.Background(), "ok/file.txt", ModeFile, []byte("y")); err != nil {
		t.Fatalf("a legitimate path was refused: %v", err)
	}

	// And nothing escaped onto disk: every file under root is under base.
	err := filepath.WalkDir(root, func(p string, _ os.DirEntry, err error) error {
		if err != nil || p == root || p == outside || p == base {
			return err
		}

		if !strings.HasPrefix(p, base+string(filepath.Separator)) {
			t.Errorf("the sink created %q outside its base", p)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
