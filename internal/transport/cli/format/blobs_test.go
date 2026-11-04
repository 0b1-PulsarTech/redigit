package format

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRowsRefusesABadManifest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		missing bool
	}{
		{name: "a missing file", content: "", missing: true},
		{name: "a broken line", content: "{", missing: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			name := filepath.Join(t.TempDir(), "m.jsonl")
			if !testCase.missing {
				if err := os.WriteFile(name, []byte(testCase.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if _, err := readRows(name); err == nil {
				t.Fatal("readRows() accepted the manifest")
			}
		})
	}
}

func TestPublishFailures(t *testing.T) {
	t.Parallel()

	const (
		lone    = `{"position":1,"blobs":[{"path":"a.go","mode":"100644","handle":"h2"}]}`
		pending = `{"position":1,"blobs":[{"path":"README.md","mode":"100644","handle":"h9"}]}`
	)

	testCases := []struct {
		name     string
		row      string
		prepare  func(t *testing.T, fix fixture)
		wantText string
	}{
		{
			name: "the out blob dir is a file",
			row:  lone,
			prepare: func(t *testing.T, fix fixture) {
				mustWrite(t, fix.outBlobs, "file")
			},
			wantText: "make out blob dir",
		},
		{
			name:     "a handle the input blob dir lacks",
			row:      pending,
			prepare:  func(t *testing.T, fix fixture) {},
			wantText: "read blob",
		},
		{
			name: "a blob path that is a directory",
			row:  lone,
			prepare: func(t *testing.T, fix fixture) {
				if err := os.MkdirAll(filepath.Join(fix.outBlobs, "h2"), 0o750); err != nil {
					t.Fatal(err)
				}
			},
			wantText: "write blob",
		},
		{
			name: "the out manifest is a directory",
			row:  lone,
			prepare: func(t *testing.T, fix fixture) {
				if err := os.Mkdir(fix.out, 0o750); err != nil {
					t.Fatal(err)
				}
			},
			wantText: "create manifest",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := stage(t, []string{testCase.row}, map[string]string{"h2": "fin"}, fakeConfig)
			testCase.prepare(t, fix)

			var stdout bytes.Buffer

			err := Run(context.Background(), fix.args(), &stdout)
			if err == nil || !strings.Contains(err.Error(), testCase.wantText) ||
				stdout.Len() != 0 {
				t.Fatalf("Run() err = %v, stdout = %q", err, stdout.String())
			}

			if stat, statErr := os.Stat(fix.out); statErr == nil && !stat.IsDir() {
				t.Fatal("the manifest was written although a blob was not")
			}
		})
	}
}
