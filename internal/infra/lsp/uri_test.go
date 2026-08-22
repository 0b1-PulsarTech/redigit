package lsp

import (
	"errors"
	"testing"
)

func TestPathToURI(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		path    string
		want    string
		wantErr error
	}{
		{
			name: "simple absolute path",
			path: "/home/user/main.go",
			want: "file:///home/user/main.go",
		},
		{
			name: "path with spaces",
			path: "/home/user/my project/main.go",
			want: "file:///home/user/my%20project/main.go",
		},
		{name: "relative path rejected", path: "main.go", wantErr: ErrRelativePath},
		{name: "empty path rejected", path: "", wantErr: ErrRelativePath},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := pathToURI(testCase.path)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && got != testCase.want {
				t.Fatalf("got = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestURIToPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		uri     string
		want    string
		wantErr error
	}{
		{name: "simple file uri", uri: "file:///home/user/main.go", want: "/home/user/main.go"},
		{
			name: "escaped space",
			uri:  "file:///home/user/my%20project/main.go",
			want: "/home/user/my project/main.go",
		},
		{
			name:    "non file scheme rejected",
			uri:     "http://example.com/main.go",
			wantErr: ErrUnsupportedScheme,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := uriToPath(testCase.uri)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && got != testCase.want {
				t.Fatalf("got = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestURIRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
	}{
		{name: "ascii", path: "/repo/internal/infra/lsp/session.go"},
		{
			name: "unicode",
			path: "/repo/pkg/日本語/main.go", //nolint:gosmopolitan // unicode fixture, not locale-sensitive UI text
		},
		{name: "spaces", path: "/repo/my module/file.go"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			uri, err := pathToURI(testCase.path)
			if err != nil {
				t.Fatalf("pathToURI: %v", err)
			}

			back, err := uriToPath(uri)
			if err != nil {
				t.Fatalf("uriToPath: %v", err)
			}

			if back != testCase.path {
				t.Fatalf("round trip = %q, want %q", back, testCase.path)
			}
		})
	}
}
