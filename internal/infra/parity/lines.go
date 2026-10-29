package parity

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	maxLineBytes     = 64 << 20
	initialLineBytes = 1 << 20

	gzipMagicLen    = 2
	gzipMagicFirst  = 0x1f
	gzipMagicSecond = 0x8b
)

var (
	ErrMalformed = errors.New("malformed fixture line")

	errStop = errors.New("stop iteration")
)

type fixtureOpener func(path string) (io.Reader, func() error, error)

type LineError struct {
	File string
	Line int
	Err  error
}

func (lineErr *LineError) Error() string {
	return fmt.Sprintf("%s:%d: %v", lineErr.File, lineErr.Line, lineErr.Err)
}

func (lineErr *LineError) Unwrap() error { return lineErr.Err }

func openFixture(path string) (io.Reader, func() error, error) {
	file, err := os.Open(path) //nolint:gosec // fixture path is chosen by the caller
	if err != nil {
		return nil, nil, fmt.Errorf("open fixture: %w", err)
	}

	buffered := bufio.NewReader(file)

	// A short file cannot be gzip, so a Peek error just means plain text.
	magic, _ := buffered.Peek(gzipMagicLen)
	if len(magic) < gzipMagicLen || magic[0] != gzipMagicFirst || magic[1] != gzipMagicSecond {
		return buffered, file.Close, nil
	}

	gz, err := gzip.NewReader(buffered)
	if err != nil {
		return nil, nil, errors.Join(
			&LineError{File: path, Err: fmt.Errorf("%w: gzip: %w", ErrMalformed, err)},
			file.Close(),
		)
	}

	return gz, func() error { return errors.Join(gz.Close(), file.Close()) }, nil
}

func forEachLine(path string, visit func(text []byte) error) error {
	return forEachLineOpen(openFixture, path, visit)
}

func forEachLineOpen(
	open fixtureOpener, path string, visit func(text []byte) error,
) (err error) {
	source, closeAll, err := open(path)
	if err != nil {
		return err
	}

	defer func() { err = errors.Join(err, closeAll()) }()

	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 0, initialLineBytes), maxLineBytes)

	lineNo := 0

	for scanner.Scan() {
		lineNo++

		text := scanner.Bytes()
		if len(text) == 0 {
			continue
		}

		visitErr := visit(text)
		if errors.Is(visitErr, errStop) {
			return nil
		}

		if visitErr != nil {
			return &LineError{File: path, Line: lineNo, Err: visitErr}
		}
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return &LineError{
			File: path, Line: lineNo + 1,
			Err: fmt.Errorf("%w: read: %w", ErrMalformed, scanErr),
		}
	}

	return nil
}
