package gitstore

import (
	"bufio"
	"fmt"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

// Identity is one author or committer line's value. When is preserved as
// given for Author and computed by the caller for Committer; fast-import
// ignores GIT_AUTHOR_DATE/GIT_COMMITTER_DATE, so the timestamp travels inside
// the stream instead.
type Identity struct {
	Name  string
	Email string
	When  time.Time
}

// FileMode is a git blob mode. It travels with every write because a tree
// hashes the mode alongside the content: rebuilding an executable script as a
// plain file produces a different tree, and the final-tree invariant then fails
// at the very end without naming the cause.
type FileMode string

const (
	ModeFile       FileMode = "100644"
	ModeExecutable FileMode = "100755"
	ModeSymlink    FileMode = "120000"
)

// Valid reports whether the mode is one git accepts for a blob. A submodule
// (160000) is not a blob and is out of scope for a rebuild that renders file
// content.
func (m FileMode) Valid() bool {
	switch m {
	case ModeFile, ModeExecutable, ModeSymlink:
		return true
	}

	return false
}

// FileWrite is one path a commit writes, with the full content that path
// holds after the commit, never a diff against a prior version. A zero Mode
// reads as ModeFile so the common case needs no ceremony.
type FileWrite struct {
	Path    string
	Mode    FileMode
	Content []byte
	Oid     string
}

func (f FileWrite) mode() FileMode {
	if f.Mode == "" {
		return ModeFile
	}

	return f.Mode
}

// WriteCommit is one commit the Writer emits. Writes and Deletes must be
// disjoint: a path present in both cannot be expressed as a single fast-import
// fileop pair unambiguously.
type WriteCommit struct {
	Author    Identity
	Committer Identity
	Message   string
	Writes    []FileWrite
	Deletes   []string
}

// formatIdentity renders one author/committer line's value: name, the email
// in angle brackets, then an explicit unix timestamp and timezone offset.
func formatIdentity(id Identity) string {
	return fmt.Sprintf("%s <%s> %d %s", id.Name, id.Email, id.When.Unix(), id.When.Format("-0700"))
}

// streamEncoder accumulates the first write error instead of returning one
// from every call, the same sticky-error shape bufio.Writer itself uses, so
// encodeCommit can read every field of a WriteCommit without an if err !=
// nil after each line.
type streamEncoder struct {
	w   *bufio.Writer
	err error
}

func (e *streamEncoder) writef(format string, args ...any) {
	if e.err != nil {
		return
	}

	_, e.err = fmt.Fprintf(e.w, format, args...)
}

// writeData renders a fast-import `data` command: an exact byte count, the
// raw bytes, then one trailing LF. The count is len(content), the encoded
// byte length, never a rune count, because an off-by-one here desyncs every
// command the stream sends after it.
func (e *streamEncoder) writeData(content []byte) {
	e.writef("data %d\n", len(content))

	if e.err != nil {
		return
	}

	if _, err := e.w.Write(content); err != nil {
		e.err = err

		return
	}

	if err := e.w.WriteByte('\n'); err != nil {
		e.err = err
	}
}

// encodeCommit renders one commit's fast-import commands onto w: the commit
// header, author/committer identity, the message, the parent link (omitted
// for a root commit, parentMark == 0), then every delete and write fileop.
// Deletes are written before writes, per commit.Deletes/commit.Writes order,
// so the same input always renders the identical byte stream.
func encodeCommit(w *bufio.Writer, ref string, mark, parentMark int, commit WriteCommit) error {
	enc := &streamEncoder{w: w}

	// A reset pins the first commit as a root whatever the ref held before.
	if parentMark == 0 {
		enc.writef("reset %s\n", ref)
	}

	enc.writef("commit %s\n", ref)
	enc.writef("mark :%d\n", mark)
	enc.writef("author %s\n", formatIdentity(commit.Author))
	enc.writef("committer %s\n", formatIdentity(commit.Committer))
	enc.writeData([]byte(commit.Message))

	if parentMark > 0 {
		enc.writef("from :%d\n", parentMark)
	}

	for _, path := range commit.Deletes {
		enc.writef("D %s\n", quotePath(path))
	}

	for _, write := range commit.Writes {
		if write.Oid != "" {
			enc.writef("M %s %s %s\n", write.mode(), write.Oid, quotePath(write.Path))

			continue
		}

		enc.writef("M %s inline %s\n", write.mode(), quotePath(write.Path))
		enc.writeData(write.Content)
	}

	return enc.err
}

func stampTime(stamp rewriting.Stamp) (time.Time, error) {
	zone, err := time.Parse("-0700", stamp.Zone)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse zone: %w", err)
	}

	_, offset := zone.Zone()

	return time.Unix(stamp.Unix, 0).In(time.FixedZone("", offset)), nil
}
