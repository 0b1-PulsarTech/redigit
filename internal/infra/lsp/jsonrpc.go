package lsp

import (
	"encoding/json"
	"errors"
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
