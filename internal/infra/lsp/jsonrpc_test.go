package lsp

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestOutgoingMessageMarshal(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		msg  outgoingMessage
		want string
	}{
		{
			name: "request has id",
			msg: outgoingMessage{
				JSONRPC: jsonrpcVersion,
				ID:      json.RawMessage("1"),
				Method:  "initialize",
				Params:  map[string]any{"a": 1},
			},
			want: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"a":1}}`,
		},
		{
			name: "notification omits id entirely",
			msg:  outgoingMessage{JSONRPC: jsonrpcVersion, Method: "exit"},
			want: `{"jsonrpc":"2.0","method":"exit"}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(testCase.msg)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			if string(got) != testCase.want {
				t.Fatalf("got = %s, want %s", got, testCase.want)
			}
		})
	}
}

func TestIncomingMessageClassification(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		body       string
		wantIsResp bool
		wantIsReq  bool
	}{
		{
			name:       "response",
			body:       `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`,
			wantIsResp: true,
		},
		{
			name:      "server request",
			body:      `{"jsonrpc":"2.0","id":7,"method":"workspace/configuration","params":{"items":[]}}`,
			wantIsReq: true,
		},
		{
			name: "notification",
			body: `{"jsonrpc":"2.0","method":"window/logMessage","params":{"message":"hi"}}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var msg incomingMessage
			if err := json.Unmarshal([]byte(testCase.body), &msg); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			isResponse := msg.Method == ""
			isServerRequest := msg.Method != "" && len(msg.ID) > 0

			if isResponse != testCase.wantIsResp {
				t.Fatalf("isResponse = %v, want %v", isResponse, testCase.wantIsResp)
			}

			if isServerRequest != testCase.wantIsReq {
				t.Fatalf("isServerRequest = %v, want %v", isServerRequest, testCase.wantIsReq)
			}
		})
	}
}

func TestResponseErrorIs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  *ResponseError
		want bool
	}{
		{
			name: "cancelled code matches",
			err:  &ResponseError{Code: cancelledCode, Message: "cancelled"},
			want: true,
		},
		{
			name: "other code does not match",
			err:  &ResponseError{Code: -32000, Message: "boom"},
			want: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := errors.Is(testCase.err, ErrCancelled); got != testCase.want {
				t.Fatalf("errors.Is = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestLocationsUnmarshal(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		body    string
		want    locations
		wantErr bool
	}{
		{name: "null", body: `null`, want: nil},
		{
			name: "single object",
			body: `{"uri":"file:///a.go","range":{"start":{"line":1,"character":2},"end":{"line":1,"character":5}}}`,
			want: locations{
				{
					URI: "file:///a.go",
					Range: Range{
						Start: Position{Line: 1, Character: 2},
						End:   Position{Line: 1, Character: 5},
					},
				},
			},
		},
		{
			name: "array of one",
			body: `[{"uri":"file:///a.go","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}}}]`,
			want: locations{
				{
					URI: "file:///a.go",
					Range: Range{
						Start: Position{Line: 0, Character: 0},
						End:   Position{Line: 0, Character: 1},
					},
				},
			},
		},
		{name: "empty array", body: `[]`, want: locations{}},
		{name: "malformed object", body: `{"uri":`, wantErr: true},
		{name: "malformed array", body: `[{"uri":`, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var got locations
			err := json.Unmarshal([]byte(testCase.body), &got)

			if testCase.wantErr {
				if err == nil {
					t.Fatalf("unmarshal: want error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if len(got) != len(testCase.want) {
				t.Fatalf("got = %+v, want %+v", got, testCase.want)
			}

			for i := range got {
				if got[i] != testCase.want[i] {
					t.Fatalf("got[%d] = %+v, want %+v", i, got[i], testCase.want[i])
				}
			}
		})
	}
}
