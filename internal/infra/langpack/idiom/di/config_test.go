package di

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

func TestDecode(t *testing.T) {
	t.Parallel()

	testCases := []decodeCase{
		{
			name: "every field",
			raw: `{"type":"go-di","registerFuncs":["Register*"],"resolveFuncs":["Get","MustGet"],` +
				`"injectorTypes":["Injector"],"roots":["main"],"paths":["cmd/"]}`,
			want: Config{
				RegisterFuncs: []string{"Register*"},
				ResolveFuncs:  []string{"Get", "MustGet"},
				InjectorTypes: []string{"Injector"},
				Roots:         []string{"main"},
				Paths:         []string{"cmd/"},
			},
		},
		{
			name: "only the register functions",
			raw:  `{"type":"go-di","registerFuncs":["Register"]}`,
			want: Config{RegisterFuncs: []string{"Register"}},
		},
		{
			name:    "no register functions",
			raw:     `{"type":"go-di","resolveFuncs":["Get"]}`,
			wantErr: ErrNoRegisterFuncs,
		},
		{
			name:    "null register functions",
			raw:     `{"type":"go-di","registerFuncs":null}`,
			wantErr: ErrNoRegisterFuncs,
		},
		{
			name:     "an unknown key",
			raw:      `{"type":"go-di","registerFuncs":["R"],"bogus":1}`,
			wantErr:  ErrConfig,
			wantText: `unknown field "bogus"`,
		},
		{
			name:     "the wrong kind of value",
			raw:      `{"type":"go-di","registerFuncs":"R"}`,
			wantErr:  ErrConfig,
			wantText: "registerFuncs",
		},
		{
			name:    "a malformed pattern",
			raw:     `{"type":"go-di","registerFuncs":["R["]}`,
			wantErr: ErrBadPattern,
		},
		{
			name:    "a malformed resolve pattern",
			raw:     `{"type":"go-di","registerFuncs":["R"],"resolveFuncs":["G[x"]}`,
			wantErr: ErrBadPattern,
		},
		{
			name:    "a malformed root pattern",
			raw:     `{"type":"go-di","registerFuncs":["R"],"roots":["[m"]}`,
			wantErr: ErrBadPattern,
		},
		{
			name:    "a pattern listed twice",
			raw:     `{"type":"go-di","registerFuncs":["R","R"]}`,
			wantErr: ErrDuplicatePattern,
		},
		{
			name:    "a malformed glob",
			raw:     `{"type":"go-di","registerFuncs":["R"],"paths":["a/[b"]}`,
			wantErr: ErrBadPattern,
		},
	}

	requireDecode(t, testCases)
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
			paths: []string{"cmd/"},
			file:  "cmd/app/main.go",
			want:  true,
		},
		{name: "a directory leaves the others", paths: []string{"cmd/"}, file: "internal/a.go"},
		{
			name:  "a glob takes a match",
			paths: []string{"cmd/*/main.go"},
			file:  "cmd/app/main.go",
			want:  true,
		},
		{name: "a glob leaves a miss", paths: []string{"cmd/*/main.go"}, file: "cmd/app/other.go"},
		{name: "one of two matches", paths: []string{"x/", "cmd/"}, file: "cmd/a.go", want: true},
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
