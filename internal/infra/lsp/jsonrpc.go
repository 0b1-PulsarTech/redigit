package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const jsonrpcVersion = "2.0"

// cancelledCode is the JSON-RPC error code a server returns for a request
// that $/cancelRequest reached before it finished. It is not a failure.
const cancelledCode = -32800

// ErrCancelled reports that a request was cancelled, either by us sending
// $/cancelRequest or by the server on its own. Callers compare with
// errors.Is, never by reading the wrapped ResponseError code themselves.
var ErrCancelled = errors.New("lsp: request cancelled")

// outgoingMessage is what this client sends: a request when ID is set, a
// notification when it is empty (omitempty drops the field entirely, which
// the spec requires for notifications).
type outgoingMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  any             `json:"params,omitempty"`
}

// incomingMessage is any message read off the wire, classified by dispatch:
// a response has Method empty, a server-to-client request has both Method
// and ID set, a notification has Method set and ID empty.
type incomingMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ResponseError is a JSON-RPC error a server returned for a request. Compare
// the sentinel ErrCancelled with errors.Is instead of reading Code directly
// for the one code (-32800) this client treats specially.
type ResponseError struct {
	Code    int
	Message string
}

func (e *ResponseError) Error() string {
	return e.Message
}

func (e *ResponseError) Is(target error) bool {
	return target == ErrCancelled && e.Code == cancelledCode
}

type cancelParams struct {
	ID json.RawMessage `json:"id"`
}

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

// jsonNull is the wire form of a JSON null value.
const jsonNull = "null"

// locations decodes a `Location | Location[] | null` result - the shape
// definition and implementation return when linkSupport is not advertised -
// into a plain slice.
type locations []Location

func (l *locations) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)

	switch {
	case len(trimmed) == 0 || string(trimmed) == jsonNull:
		*l = nil

		return nil
	case trimmed[0] == '[':
		var arr []Location
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return fmt.Errorf("unmarshal locations array: %w", err)
		}

		*l = arr

		return nil
	default:
		var single Location
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return fmt.Errorf("unmarshal single location: %w", err)
		}

		*l = locations{single}

		return nil
	}
}
