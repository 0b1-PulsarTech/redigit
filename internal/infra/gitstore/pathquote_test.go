package gitstore

import (
	"errors"
	"strings"
	"testing"
)

func TestQuotePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want string
	}{
		{"plain path unchanged", "internal/foo/bar.go", "internal/foo/bar.go"},
		{"empty path unchanged", "", ""},
		{"space forces quoting", "dir with space/b.go", `"dir with space/b.go"`},
		{"embedded quote is escaped", `a"b.go`, `"a\"b.go"`},
		{"embedded backslash is escaped", `a\b.go`, `"a\\b.go"`},
		{"control byte is octal escaped", "a\tb.go", `"a\011b.go"`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := quotePath(testCase.path); got != testCase.want {
				t.Fatalf("quotePath(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		path    string
		isValid bool
	}{
		{"plain path", "internal/foo/bar.go", true},
		{"dotfile", ".gitignore", true},
		{"directory that merely starts with .git", ".github/workflows/ci.yml", true},
		{"name that merely contains .git", "a.git/x", true},
		{"empty", "", false},
		{"dot component", "a/./b", false},
		{"dot-dot component", "a/../b", false},
		{"leading dot-dot", "../b", false},
		{"lone dot", ".", false},
		{"git dir", ".git/config", false},
		{"git dir in upper case", ".GIT/config", false},
		{"git dir in mixed case", "a/.Git/hooks/x", false},
		{"nested git file", "a/b/.git", false},
		{"leading slash", "/a/b", false},
		{"trailing slash", "a/b/", false},
		{"double slash", "a//b", false},
		{"nul byte hiding a git dir", ".git\x00x", false},
		{"nul byte in a plain name", "a\x00b", false},
		{"ntfs trailing space", ".git /config", false},
		{"ntfs trailing dot", "a/.git./config", false},
		{"ntfs trailing dots and spaces", ".GIT . ./x", false},
		{"ntfs short name", "GIT~1/config", false},
		{"ntfs short name with trailing space", "a/git~1 /x", false},
		{"hfs zero width non-joiner", ".g\xe2\x80\x8cit/config", false},
		{"hfs zero width joiner first", "\xe2\x80\x8d.git/config", false},
		{"hfs marks", ".git\xe2\x80\x8e\xe2\x80\x8f/x", false},
		{"hfs embedding controls", ".gi\xe2\x80\xaat/x", false},
		{"hfs deprecated format", ".\xe2\x81\xafgit/x", false},
		{"hfs byte order mark", ".git\xef\xbb\xbf", false},
		{"ntfs stream suffix", ".git:x", false},
		{"ntfs index allocation stream", ".GIT::$INDEX_ALLOCATION", false},
		{"ntfs backslash separated git dir", "a\\.git\\hooks\\post-checkout", false},
		{"ntfs spaces and dots before a stream", ".git . :y", false},
		{"ntfs short name stream", "git~1:x/config", false},
		{"ntfs colon before a backslash separator", "x:y\\.git", false},
		{"other short name", "git~2/x", true},
		{"git prefix", "gitx/x", true},
		{"unrelated zero width", "a\xe2\x80\x8cb", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := validatePath(testCase.path)
			if testCase.isValid {
				if err != nil {
					t.Fatalf("validatePath(%q) err = %v, want nil", testCase.path, err)
				}

				return
			}

			badPath, isBad := errors.AsType[*BadPathError](err)
			if !isBad || badPath.Path != testCase.path || !errors.Is(err, errBadPath) {
				t.Fatalf("validatePath(%q) err = %v, want a BadPathError", testCase.path, err)
			}
		})
	}
}

func TestValidateCommitRefusesABadIdentity(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		who     Identity
		wantErr error
	}{
		{"plain", Identity{Name: "A B", Email: "a@b.c"}, nil},
		{"empty", Identity{}, nil},
		{"newline in name", Identity{Name: "A\ncommit x", Email: "a@b.c"}, ErrBadIdentity},
		{"angle in name", Identity{Name: "A <x>", Email: "a@b.c"}, ErrBadIdentity},
		{"angle in email", Identity{Name: "A", Email: "a>b"}, ErrBadIdentity},
		{"carriage return", Identity{Name: "A\r", Email: "a@b.c"}, ErrBadIdentity},
		{"control byte in email", Identity{Name: "A", Email: "a\x01b"}, ErrBadIdentity},
		{"nul in name", Identity{Name: "A\x00", Email: "a@b.c"}, ErrBadIdentity},
	}

	for _, testCase := range testCases {
		for _, role := range []string{"author", "committer"} {
			t.Run(testCase.name+" "+role, func(t *testing.T) {
				t.Parallel()

				commit := WriteCommit{Author: Identity{Name: "ok", Email: "ok@x"}}
				if role == "author" {
					commit.Author = testCase.who
				} else {
					commit.Committer = testCase.who
				}

				if err := validateCommit(commit); !errors.Is(err, testCase.wantErr) {
					t.Fatalf("validateCommit() err = %v, want %v", err, testCase.wantErr)
				}
			})
		}
	}
}

func TestValidateCommitRefusesAGitmodulesSymlink(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		path    string
		mode    FileMode
		isValid bool
	}{
		{"symlink", ".gitmodules", ModeSymlink, false},
		{"symlink in a directory", "a/b/.gitmodules", ModeSymlink, false},
		{"symlink upper case", ".GitModules", ModeSymlink, false},
		{"symlink trailing dots and spaces", ".gitmodules . .", ModeSymlink, false},
		{"symlink ntfs short name", "GITMOD~1", ModeSymlink, false},
		{"symlink ntfs short name with space", "a/gitmod~1 ", ModeSymlink, false},
		{"symlink ntfs stream", ".gitmodules:x", ModeSymlink, false},
		{"symlink spaces before a stream", ".gitmodules . :y", ModeSymlink, false},
		{"symlink hfs ignorable code point", ".git\xe2\x80\x8cmodules", ModeSymlink, false},
		{"symlink hfs byte order mark", "\xef\xbb\xbf.gitmodules", ModeSymlink, false},
		{"regular file", ".gitmodules", ModeFile, true},
		{"regular file by default mode", ".gitmodules", "", true},
		{"executable file", "a/.gitmodules", ModeExecutable, true},
		{"regular file ntfs spelling", ".gitmodules:x", ModeFile, true},
		{"symlink with another name", "gitmodules", ModeSymlink, true},
		{"symlink other short name", "gitmod~2", ModeSymlink, true},
		{"symlink below a directory of that name", ".gitmodules/x", ModeSymlink, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := WriteCommit{Writes: []FileWrite{
				{Path: testCase.path, Mode: testCase.mode, Content: []byte("x")},
			}}

			err := validateCommit(commit)
			if testCase.isValid {
				if err != nil {
					t.Fatalf("validateCommit(%q) err = %v, want nil", testCase.path, err)
				}

				return
			}

			badPath, isBad := errors.AsType[*BadPathError](err)
			if !isBad || badPath.Path != testCase.path {
				t.Fatalf("validateCommit(%q) err = %v, want a BadPathError", testCase.path, err)
			}
		})
	}
}

func TestBadPathErrorNamesThePath(t *testing.T) {
	t.Parallel()

	err := validateCommit(WriteCommit{Writes: []FileWrite{{Path: "a/../b"}}})
	if err == nil || !strings.Contains(err.Error(), `"a/../b"`) {
		t.Fatalf("validateCommit() err = %v, want one quoting the path", err)
	}
}

func TestValidateCommitRefusesABadWritePath(t *testing.T) {
	t.Parallel()

	commit := WriteCommit{Writes: []FileWrite{{Path: "a/../b", Content: []byte("x")}}}

	if _, isBad := errors.AsType[*BadPathError](validateCommit(commit)); !isBad {
		t.Fatalf("validateCommit() did not refuse a dot-dot path")
	}
}
