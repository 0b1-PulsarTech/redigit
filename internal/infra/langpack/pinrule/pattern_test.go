package pinrule

import (
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestParsePattern(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		text    string
		wantErr error
	}{
		{name: "a literal key", text: "apps/x/y.go:func:Z"},
		{name: "globs in every part", text: "apps/*/y_test.go:*:Test*"},
		{name: "a name that holds a colon", text: "a.go:value:x:y"},
		{name: "two parts only", text: "a.go:Z", wantErr: ErrBadPattern},
		{name: "a bare file", text: "a.go", wantErr: ErrBadPattern},
		{name: "an empty file", text: ":func:Z", wantErr: ErrBadPattern},
		{name: "an empty kind", text: "a.go::Z", wantErr: ErrBadPattern},
		{name: "an empty name", text: "a.go:func:", wantErr: ErrBadPattern},
		{name: "a malformed glob", text: "a[.go:func:Z", wantErr: ErrBadPattern},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := parsePattern(testCase.text)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("parsePattern(%q) err = %v, want %v", testCase.text, err, testCase.wantErr)
			}
		})
	}
}

func TestPatternMatches(t *testing.T) {
	t.Parallel()

	key := entities.DeclKey{File: "apps/x/y.go", Kind: entities.KindFunc, Name: "Zed"}
	testCases := []struct {
		name string
		text string
		want bool
	}{
		{name: "the exact key", text: "apps/x/y.go:func:Zed", want: true},
		{name: "a symbol glob", text: "apps/x/y.go:func:Z*", want: true},
		{name: "a file glob", text: "apps/*/y.go:func:Zed", want: true},
		{name: "any kind", text: "apps/x/y.go:*:Zed", want: true},
		{name: "another file", text: "apps/x/z.go:func:Zed"},
		{name: "another kind", text: "apps/x/y.go:type:Zed"},
		{name: "another symbol", text: "apps/x/y.go:func:Zee"},
		{name: "a star does not cross a directory", text: "apps/*.go:func:Zed"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pat, err := parsePattern(testCase.text)
			if err != nil {
				t.Fatalf("parsePattern(%q) = %v", testCase.text, err)
			}

			if got := pat.matches(key); got != testCase.want {
				t.Fatalf("%q matches = %v, want %v", testCase.text, got, testCase.want)
			}
		})
	}
}
