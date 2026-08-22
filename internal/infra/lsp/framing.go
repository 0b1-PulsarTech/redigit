package lsp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ErrMissingContentLength reports a frame header with no Content-Length field,
// which is the one header the LSP framing protocol requires.
var ErrMissingContentLength = errors.New("lsp: missing content-length header")

// writeFrame writes body as one LSP message: a Content-Length header, a blank
// line, then the raw bytes, exactly as the stdio transport requires.
func writeFrame(w io.Writer, body []byte) error {
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}

	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write frame body: %w", err)
	}

	return nil
}

// readFrame reads one LSP message from r: a sequence of "Name: value" header
// lines terminated by a blank line, then exactly Content-Length body bytes.
// Unknown headers (Content-Type is the only other one servers send) are
// skipped.
func readFrame(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read frame header: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}

		if !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}

		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("parse content-length: %w", err)
		}

		contentLength = n
	}

	if contentLength < 0 {
		return nil, ErrMissingContentLength
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read frame body: %w", err)
	}

	return body, nil
}
