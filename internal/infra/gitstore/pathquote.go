package gitstore

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// quoteOverhead is the two double-quote bytes a quoted path adds around its
// (possibly escaped) content.
const quoteOverhead = 2

// quotePath renders path the way fast-import needs it on an M/D command
// line: unquoted when plain, or C-style quoted when it holds a space, a
// quote, a backslash, or a control byte. An unquoted space was measured to
// make git fast-import reject the whole line as an unsupported command.
func quotePath(path string) string {
	if !needsQuote(path) {
		return path
	}

	quoted := make([]byte, 0, len(path)+quoteOverhead)
	quoted = append(quoted, '"')

	for i := range len(path) {
		quoted = appendQuotedByte(quoted, path[i])
	}

	quoted = append(quoted, '"')

	return string(quoted)
}

func appendQuotedByte(quoted []byte, b byte) []byte {
	switch {
	case b == '"' || b == '\\':
		return append(quoted, '\\', b)
	case b < 0x20 || b == 0x7f:
		return fmt.Appendf(quoted, `\%03o`, b)
	default:
		return append(quoted, b)
	}
}

func needsQuote(path string) bool {
	for i := range len(path) {
		b := path[i]
		if b == ' ' || b == '"' || b == '\\' || b < 0x20 || b == 0x7f {
			return true
		}
	}

	return false
}

var ErrBadIdentity = errors.New("bad author or committer identity")

func validateCommit(commit WriteCommit) error {
	// A line break or angle bracket would end the identity line and let the rest be read
	// as another fast-import command.
	for _, who := range []Identity{commit.Author, commit.Committer} {
		for _, field := range []string{who.Name, who.Email} {
			isBad := strings.ContainsAny(field, "<>") ||
				strings.IndexFunc(field, unicode.IsControl) >= 0
			if isBad {
				return fmt.Errorf("%w: %q", ErrBadIdentity, field)
			}
		}
	}

	deleted := make(map[string]bool, len(commit.Deletes))
	for _, path := range commit.Deletes {
		deleted[path] = true
	}

	for _, write := range commit.Writes {
		if deleted[write.Path] {
			return &PathConflictError{Path: write.Path}
		}

		if err := validatePath(write.Path); err != nil {
			return err
		}

		if isGitmodulesLink(write.Path, write.mode()) {
			return &BadPathError{Path: write.Path}
		}

		if !write.mode().Valid() {
			return &UnsupportedModeError{Path: write.Path, Mode: write.Mode}
		}
	}

	return nil
}

var errBadPath = errors.New("unsafe repository path")

type BadPathError struct {
	Path string
}

func (e *BadPathError) Error() string { return fmt.Sprintf("unsafe repository path %q", e.Path) }

func (e *BadPathError) Unwrap() error { return errBadPath }

func validatePath(path string) error {
	if path == "" || strings.ContainsRune(path, 0) {
		return &BadPathError{Path: path}
	}

	for component := range strings.SplitSeq(path, "/") {
		if component == "" || component == "." || component == ".." ||
			isGitDirAlias(component) {
			return &BadPathError{Path: path}
		}
	}

	return nil
}

func isGitDirAlias(component string) bool {
	// Follows git's is_ntfs_dotgit and is_hfs_dotgit: a checkout on those filesystems
	// resolves these spellings to the .git directory.
	// NTFS reads a backslash as a separator, so each piece is judged on its own.
	for piece := range strings.SplitSeq(component, `\`) {
		if isNTFSName(piece, ".git", "git~1") {
			return true
		}
	}

	return isHFSName(component, ".git")
}

func isGitmodulesLink(path string, mode FileMode) bool {
	// Follows git's verify_dotfile: a symlink named .gitmodules is read as the submodule
	// config, so its target would be followed on checkout.
	name := path[strings.LastIndexByte(path, '/')+1:]

	return mode == ModeSymlink &&
		(isNTFSName(name, ".gitmodules", "gitmod~1") || isHFSName(name, ".gitmodules"))
}

func isNTFSName(piece, name, short string) bool {
	// NTFS ends a name at a stream colon and ignores trailing dots and spaces.
	stream, _, _ := strings.Cut(piece, ":")
	ntfs := strings.TrimRight(stream, ". ")

	return strings.EqualFold(ntfs, name) || strings.EqualFold(ntfs, short)
}

func isHFSName(component, name string) bool {
	const byteOrderMark = 0xfeff

	hfs := strings.Map(func(char rune) rune {
		isIgnorable := char >= 0x200c && char <= 0x200f || char >= 0x202a && char <= 0x202e ||
			char >= 0x206a && char <= 0x206f || char == byteOrderMark
		if isIgnorable {
			return -1
		}

		return char
	}, component)

	return strings.EqualFold(hfs, name)
}
