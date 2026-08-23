package gitstore

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// Shape is what kind of commit a manifest line describes.
type Shape string

const (
	// ShapeFeature is a commit that stands on its own as one feature slice.
	ShapeFeature Shape = "feature"
	// ShapeLayer is a cross-cutting step (scaffolding, a shared schema) with
	// no single feature to name.
	ShapeLayer Shape = "layer"
)

// Chunk is one declaration born in this commit: its stable key and the byte
// span it occupies in the blob it belongs to. The agent has the final source
// in hand, so a span is enough to see the exact body; the text travels
// separately only when asked for, because duplicating megabytes per commit
// would not pay for itself.
type Chunk struct {
	Key  string `json:"key"`
	Span [2]int `json:"span"`
}

// Blob is one file's whole content as it stands after this commit, never a
// diff against a prior version. OID is the object id a BlobStore assigned; it
// is empty, and omitted, when the manifest was built against a DirSink, which
// has no object database to assign one from.
type Blob struct {
	Path string   `json:"path"`
	OID  string   `json:"oid,omitempty"`
	Mode FileMode `json:"mode"`
}

// Risk is a reference a chunk carries that points at a declaration this
// manifest has not born yet, at BornAt. The tool reports it without judging
// whether the commit still compiles; the agent has go build and the source to
// decide that.
type Risk struct {
	Chunk     string `json:"chunk"`
	Reference string `json:"reference"`
	BornAt    int    `json:"born_at"`
}

// Commit is one line of a materialized plan: everything an orchestrating
// agent needs to stage and commit one step, without the tool ever writing
// history itself.
type Commit struct {
	Position int     `json:"position"`
	Shape    Shape   `json:"shape"`
	Feature  string  `json:"feature,omitempty"`
	Subject  string  `json:"subject"`
	Chunks   []Chunk `json:"chunks"`
	Blobs    []Blob  `json:"blobs"`
	Risks    []Risk  `json:"risks"`
}

// InvalidUTF8Error is a commit whose Subject, Feature, Chunk.Key, Blob.Path,
// or Risk.Chunk/Reference field holds invalid UTF-8. encoding/json does not
// error on invalid UTF-8 in a Go string; it silently replaces the bad bytes
// with U+FFFD, so EncodeManifest refuses the whole commit up front instead
// of letting json.Encoder corrupt it without a signal.
type InvalidUTF8Error struct {
	Position int
	Field    string
}

func (e *InvalidUTF8Error) Error() string { return "manifest field is not valid utf-8" }

func (e *InvalidUTF8Error) Unwrap() error { return errInvalidUTF8 }

var errInvalidUTF8 = errors.New("invalid utf-8")

// ManifestLineError is one manifest line that failed to decode as JSON. Line
// carries the failing line's 1-based position, and Unwrap exposes the
// underlying json.Unmarshal error, so a caller can errors.As/errors.Is
// against both without parsing the message text.
type ManifestLineError struct {
	Line  int
	cause error
}

func (e *ManifestLineError) Error() string { return "manifest line failed to decode" }

func (e *ManifestLineError) Unwrap() error { return e.cause }

// EncodeManifest writes one Commit per line as JSON Lines, so an agent can
// stream a plan instead of holding the whole thing in memory.
//
// Every commit's string fields are validated as UTF-8 before any line is
// written, so a bad commit anywhere in commits leaves the writer untouched
// rather than emitting a partial, silently-corrupted manifest.
func EncodeManifest(w io.Writer, commits []Commit) error {
	for _, commit := range commits {
		if field := firstInvalidUTF8Field(commit); field != "" {
			return &InvalidUTF8Error{Position: commit.Position, Field: field}
		}
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // this is a human/grep-readable JSONL file, not an HTML sink

	for _, commit := range commits {
		withEmptySlices(&commit)

		if err := enc.Encode(commit); err != nil {
			return fmt.Errorf("encode manifest line at position %d: %w", commit.Position, err)
		}
	}

	return nil
}

// firstInvalidUTF8Field reports the name of the first string field in commit
// that is not valid UTF-8, or "" when every one of them is.
func firstInvalidUTF8Field(commit Commit) string {
	if !utf8.ValidString(commit.Subject) {
		return "subject"
	}

	if !utf8.ValidString(commit.Feature) {
		return "feature"
	}

	for i, chunk := range commit.Chunks {
		if !utf8.ValidString(chunk.Key) {
			return fmt.Sprintf("chunks[%d].key", i)
		}
	}

	for i, blob := range commit.Blobs {
		if !utf8.ValidString(blob.Path) {
			return fmt.Sprintf("blobs[%d].path", i)
		}
	}

	for i, risk := range commit.Risks {
		if !utf8.ValidString(risk.Chunk) {
			return fmt.Sprintf("risks[%d].chunk", i)
		}

		if !utf8.ValidString(risk.Reference) {
			return fmt.Sprintf("risks[%d].reference", i)
		}
	}

	return ""
}

// withEmptySlices normalizes commit's nil Chunks/Blobs/Risks to an empty,
// non-nil slice so it serializes as "[]" rather than "null": a DTO slice
// field crossing a JSON boundary must not surprise a consumer expecting an
// array.
func withEmptySlices(commit *Commit) {
	if commit.Chunks == nil {
		commit.Chunks = []Chunk{}
	}

	if commit.Blobs == nil {
		commit.Blobs = []Blob{}
	}

	if commit.Risks == nil {
		commit.Risks = []Risk{}
	}
}

// manifestScanBuffer is the initial buffer bufio.Scanner grows from; it stays
// small because most manifest lines are far shorter than the 1 MiB cap below.
const manifestScanBuffer = 64 * 1024

// manifestMaxLine bounds a single manifest line: a commit touching many large
// files can still produce a long line, but an unbounded one would let a
// malformed manifest exhaust memory before DecodeManifest ever reports an error.
const manifestMaxLine = 1024 * 1024

// DecodeManifest reads a JSON Lines manifest back into its commits, in the
// order they were written.
func DecodeManifest(r io.Reader) ([]Commit, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, manifestScanBuffer), manifestMaxLine)

	var (
		commits []Commit
		lineNum int
	)

	for scanner.Scan() {
		lineNum++

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var commit Commit
		if err := json.Unmarshal(line, &commit); err != nil {
			return nil, &ManifestLineError{Line: lineNum, cause: err}
		}

		commits = append(commits, commit)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	return commits, nil
}
