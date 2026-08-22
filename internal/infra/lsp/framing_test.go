package lsp

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestWriteFrame(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := writeFrame(&buf, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}

	want := "Content-Length: 7\r\n\r\n{\"a\":1}"
	if got := buf.String(); got != want {
		t.Fatalf("got = %q, want %q", got, want)
	}
}

func TestReadFrame(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		input            string
		want             string
		wantErr          bool
		wantMissingCause bool
	}{
		{
			name:  "simple body",
			input: "Content-Length: 7\r\n\r\n{\"a\":1}",
			want:  `{"a":1}`,
		},
		{
			name:  "extra header ignored",
			input: "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\nContent-Length: 4\r\n\r\nabcd",
			want:  "abcd",
		},
		{
			name:  "header casing and spacing",
			input: "content-length:   4\r\n\r\nabcd",
			want:  "abcd",
		},
		{
			name:             "missing content length",
			input:            "Content-Type: text\r\n\r\nabcd",
			wantErr:          true,
			wantMissingCause: true,
		},
		{
			name:    "truncated body",
			input:   "Content-Length: 10\r\n\r\nabcd",
			wantErr: true,
		},
		{
			name:    "malformed length",
			input:   "Content-Length: nope\r\n\r\nabcd",
			wantErr: true,
		},
		{
			name:    "no headers at all",
			input:   "",
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			reader := bufio.NewReader(strings.NewReader(testCase.input))
			got, err := readFrame(reader)

			if testCase.wantErr {
				if err == nil {
					t.Fatalf("readFrame: want error, got nil")
				}

				if testCase.wantMissingCause && !errors.Is(err, ErrMissingContentLength) {
					t.Fatalf("err = %v, want %v", err, ErrMissingContentLength)
				}

				return
			}

			if err != nil {
				t.Fatalf("readFrame: %v", err)
			}

			if string(got) != testCase.want {
				t.Fatalf("got = %q, want %q", got, testCase.want)
			}
		})
	}
}
