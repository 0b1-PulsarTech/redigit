package batch

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"strconv"
	"strings"
	"time"
)

var ErrMalformedLog = errors.New("malformed git log")

type (
	Entry struct {
		OldMode, NewMode string
		OldBlob, NewBlob string
		Status           byte
		Similarity       int
		OldPath, NewPath string
	}

	Commit struct {
		OID        string
		AuthorDate time.Time
		Entries    []Entry
	}
)

func (entry Entry) IsDelete() bool { return entry.Status == 'D' }

func ParseLog(source io.Reader) iter.Seq2[Commit, error] {
	return func(yield func(Commit, error) bool) {
		parser := logParser{reader: bufio.NewReaderSize(source, pipeBuffer)}

		for {
			commit, isDone, err := parser.next()
			if err != nil {
				yield(Commit{}, err)

				return
			}

			if isDone || !yield(commit, nil) {
				return
			}
		}
	}
}

type logParser struct {
	reader  *bufio.Reader
	pending Commit
	isOpen  bool
}

func (parser *logParser) next() (commit Commit, isDone bool, err error) {
	for {
		token, readErr := parser.reader.ReadBytes(0)
		if errors.Is(readErr, io.EOF) && len(token) == 0 {
			return parser.flush()
		}

		if readErr != nil {
			return Commit{}, false, fmt.Errorf("%w: unterminated: %w", ErrMalformedLog, readErr)
		}

		token = bytes.TrimSuffix(token, []byte{0})

		if !bytes.HasPrefix(token, []byte{1}) {
			if err = parser.addEntry(string(bytes.TrimLeft(token, "\n"))); err != nil {
				return Commit{}, false, err
			}

			continue
		}

		header, headerErr := parseHeader(string(token[1:]))
		if headerErr != nil {
			return Commit{}, false, headerErr
		}

		finished, wasOpen := parser.pending, parser.isOpen
		parser.pending, parser.isOpen = header, true

		if wasOpen {
			return finished, false, nil
		}
	}
}

func (parser *logParser) flush() (commit Commit, isDone bool, err error) {
	if !parser.isOpen {
		return Commit{}, true, nil
	}

	parser.isOpen = false

	return parser.pending, false, nil
}

func (parser *logParser) addEntry(meta string) error {
	if !parser.isOpen {
		return fmt.Errorf("%w: entry before any commit header", ErrMalformedLog)
	}

	entry, err := parseEntry(parser.reader, meta)
	if err != nil {
		return err
	}

	parser.pending.Entries = append(parser.pending.Entries, entry)

	return nil
}

func parseHeader(text string) (Commit, error) {
	oid, stamp, found := strings.Cut(text, " ")
	if !found || oid == "" {
		return Commit{}, fmt.Errorf("%w: commit header %q", ErrMalformedLog, text)
	}

	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return Commit{}, fmt.Errorf("%w: author date: %w", ErrMalformedLog, err)
	}

	return Commit{OID: oid, AuthorDate: time.Unix(seconds, 0).UTC()}, nil
}
