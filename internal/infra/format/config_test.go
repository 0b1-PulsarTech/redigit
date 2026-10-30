package format

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestDecode(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		wantErr bool
		want    time.Duration
	}{
		{
			name: "a full config",
			input: `{"languages":[{"name":"go","extensions":[".go"],"steps":[` +
				`{"cmd":["gofumpt","-w"],"mode":"files","timeout":"30s"}]}],"workers":2}`,
			want: 30 * time.Second,
		},
		{name: "an unknown field", input: `{"nope":1}`, wantErr: true},
		{
			name:    "a numeric timeout",
			input:   `{"languages":[{"steps":[{"cmd":["x"],"mode":"files","timeout":30}]}]}`,
			wantErr: true,
		},
		{
			name:    "an unparsable timeout",
			input:   `{"languages":[{"steps":[{"cmd":["x"],"mode":"files","timeout":"soon"}]}]}`,
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config, err := Decode(strings.NewReader(testCase.input))
			if (err != nil) != testCase.wantErr {
				t.Fatalf("error = %v, want error %v", err, testCase.wantErr)
			}

			if testCase.wantErr {
				return
			}

			if got := time.Duration(config.Languages[0].Steps[0].Timeout); got != testCase.want {
				t.Errorf("timeout = %s, want %s", got, testCase.want)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	failure := &Failure{
		Tool:   "gofumpt",
		Path:   "a.go",
		Stderr: "boom",
		Err:    context.DeadlineExceeded,
	}
	missing := &MissingToolError{Tool: "nope", Err: context.Canceled}

	testCases := []struct {
		name     string
		err      error
		contains []string
		wraps    error
	}{
		{"a failure", failure, []string{"a.go", "gofumpt", "boom"}, context.DeadlineExceeded},
		{"a missing tool", missing, []string{"nope"}, context.Canceled},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			for _, part := range testCase.contains {
				if !strings.Contains(testCase.err.Error(), part) {
					t.Errorf("%q lacks %q", testCase.err, part)
				}
			}

			if !errors.Is(testCase.err, testCase.wraps) {
				t.Errorf("%v does not wrap %v", testCase.err, testCase.wraps)
			}
		})
	}
}

func TestAnchored(t *testing.T) {
	root := t.TempDir()

	testCases := []struct {
		name string
		cmd  string
		root string
		want string
	}{
		{"a bare name goes to PATH", "gofumpt", root, "gofumpt"},
		{"an absolute path is kept", "/usr/bin/x", root, "/usr/bin/x"},
		{
			"a relative path hangs off the root",
			"./bin/fmt",
			root,
			filepath.Join(root, "bin", "fmt"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := anchored(testCase.cmd, testCase.root); got != testCase.want {
				t.Errorf("anchored = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestNewFailure(t *testing.T) {
	testCases := []struct {
		name   string
		stderr string
		want   string
	}{
		{"short text is kept and trimmed", "  boom \n", "boom"},
		{"long ascii is cut at the limit", strings.Repeat("a", 300), strings.Repeat("a", 240)},
		{
			"a multibyte rune is not cut in half",
			"a" + strings.Repeat(string(rune(0xe9)), 200),
			"a" + strings.Repeat(string(rune(0xe9)), 119),
		},
		{
			"a three byte rune on the edge is dropped whole",
			"a" + strings.Repeat(string(rune(0x1f600)), 100),
			"a" + strings.Repeat(string(rune(0x1f600)), 59),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			failure := newFailure(tool{name: "x"}, "a.go", testCase.stderr, ErrConfig)

			if failure.Stderr != testCase.want || !utf8.ValidString(failure.Stderr) {
				t.Errorf("excerpt = %q, want %q", failure.Stderr, testCase.want)
			}
		})
	}
}
