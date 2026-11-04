package router

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type decodeCase struct {
	name     string
	raw      string
	want     Config
	wantErr  error
	wantText string
}

func requireDecode(t *testing.T, testCases []decodeCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := decode(json.RawMessage(testCase.raw))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("decode() err = %v, want %v", err, testCase.wantErr)
			}

			if err != nil && !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("decode() err = %q, want it to hold %q", err, testCase.wantText)
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("decode() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	t.Parallel()

	testCases := []decodeCase{
		{
			name: "every field",
			raw: `{"type":"go-router","handle":["*.GET","*.POST"],"mount":["Mount*"],` +
				`"groupMethods":true,"paths":["api/"]}`,
			want: Config{
				Handle:       []string{"*.GET", "*.POST"},
				Mount:        []string{"Mount*"},
				GroupMethods: true,
				Paths:        []string{"api/"},
			},
		},
		{
			name: "only the handle patterns",
			raw:  `{"type":"go-router","handle":["*.GET"]}`,
			want: Config{Handle: []string{"*.GET"}},
		},
		{
			name:    "no handle patterns",
			raw:     `{"type":"go-router","mount":["Mount*"]}`,
			wantErr: ErrNoHandle,
		},
		{
			name:    "null handle patterns",
			raw:     `{"type":"go-router","handle":null}`,
			wantErr: ErrNoHandle,
		},
		{
			name:     "an unknown key",
			raw:      `{"type":"go-router","handle":["*.GET"],"bogus":1}`,
			wantErr:  ErrConfig,
			wantText: `unknown field "bogus"`,
		},
		{
			name:     "the wrong kind of value",
			raw:      `{"type":"go-router","handle":"*.GET"}`,
			wantErr:  ErrConfig,
			wantText: "handle",
		},
		{
			name:     "groupMethods as a list",
			raw:      `{"type":"go-router","handle":["*.GET"],"groupMethods":["x"]}`,
			wantErr:  ErrConfig,
			wantText: "groupMethods",
		},
	}

	requireDecode(t, testCases)
}

func TestDecodePatterns(t *testing.T) {
	t.Parallel()

	testCases := []decodeCase{
		{
			name:    "a malformed handle pattern",
			raw:     `{"type":"go-router","handle":["*.GE["]}`,
			wantErr: ErrBadPattern,
		},
		{
			name:    "a malformed mount pattern",
			raw:     `{"type":"go-router","handle":["*.GET"],"mount":["M["]}`,
			wantErr: ErrBadPattern,
		},
		{
			name:    "a malformed glob",
			raw:     `{"type":"go-router","handle":["*.GET"],"paths":["a/[b"]}`,
			wantErr: ErrBadPattern,
		},
		{
			name:    "a handle pattern listed twice",
			raw:     `{"type":"go-router","handle":["*.GET","*.GET"]}`,
			wantErr: ErrDuplicatePattern,
		},
		{
			name:    "a mount pattern listed twice",
			raw:     `{"type":"go-router","handle":["*.GET"],"mount":["M*","M*"]}`,
			wantErr: ErrDuplicatePattern,
		},
	}

	requireDecode(t, testCases)
}

func TestConfigWithinPaths(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		paths []string
		file  string
		want  bool
	}{
		{name: "no paths takes every file", file: "any/where.go", want: true},
		{
			name:  "a directory takes its files",
			paths: []string{"api/"},
			file:  "api/v1/a.go",
			want:  true,
		},
		{name: "a directory leaves the others", paths: []string{"api/"}, file: "internal/a.go"},
		{name: "a directory is no name prefix", paths: []string{"api/"}, file: "apix/a.go"},
		{
			name:  "a directory is no suffix of a deeper one",
			paths: []string{"api/"},
			file:  "x/api/a.go",
		},
		{
			name:  "a directory glob takes a match",
			paths: []string{"*/api/"},
			file:  "x/api/a.go",
			want:  true,
		},
		{
			name:  "a file glob takes a match",
			paths: []string{"api/*.go"},
			file:  "api/a.go",
			want:  true,
		},
		{name: "a file glob stops at a slash", paths: []string{"api/*.go"}, file: "api/v1/a.go"},
		{name: "one of two matches", paths: []string{"x/", "api/"}, file: "api/a.go", want: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := Config{Paths: testCase.paths}
			if got := cfg.within(testCase.file); got != testCase.want {
				t.Fatalf("within(%q) = %t, want %t", testCase.file, got, testCase.want)
			}
		})
	}
}
