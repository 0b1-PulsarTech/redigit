package format

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

type guardCase struct {
	name     string
	prepare  func(t *testing.T, fix fixture, dir string) []string
	wantText string
}

func TestRefuseOverwriteOut(t *testing.T) {
	t.Parallel()

	const wantText = "--out must not be its input"

	testCases := []guardCase{
		{
			name: "a symlink to the manifest",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				return []string{"--out", linkTo(t, fix.manifest, dir, "m.link")}
			},
			wantText: wantText,
		},
		{
			name: "a hard link to the manifest",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				hard := filepath.Join(dir, "m.hard")
				if err := os.Link(fix.manifest, hard); err != nil {
					t.Fatal(err)
				}

				return []string{"--out", hard}
			},
			wantText: wantText,
		},
		{
			name: "the config",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				return []string{"--out", fix.config}
			},
			wantText: wantText,
		},
		{
			name: "a symlink to the config",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				return []string{"--out", linkTo(t, fix.config, dir, "c.link")}
			},
			wantText: wantText,
		},
		{
			name: "a new file inside the blob dir",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				return []string{"--out", filepath.Join(fix.blobs, "f.json")}
			},
			wantText: wantText,
		},
		{
			name: "a symlink inside the blob dir to the manifest",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				return []string{"--out", linkTo(t, fix.manifest, fix.blobs, "h2")}
			},
			wantText: wantText,
		},
		{
			name: "the blob dir",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				return []string{"--out", fix.blobs}
			},
			wantText: wantText,
		},
		{
			name: "a path that does not exist yet under a symlink to the blob dir",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				deep := filepath.Join(linkTo(t, fix.blobs, dir, "b.link"), "deep", "m.json")

				return []string{"--out", deep}
			},
			wantText: wantText,
		},
		{
			name: "a file inside the out blob dir",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				return []string{"--out", filepath.Join(fix.outBlobs, "m.json")}
			},
			wantText: wantText,
		},
	}

	runGuard(t, testCases)
}

func TestRefuseOverwriteOutBlobDir(t *testing.T) {
	t.Parallel()

	const wantText = "--out-blob-dir must not be its input"

	testCases := []guardCase{
		{
			name: "a symlink to the blob dir",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				return []string{"--out-blob-dir", linkTo(t, fix.blobs, dir, "b.link")}
			},
			wantText: wantText,
		},
		{
			name: "a directory inside the blob dir",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				return []string{"--out-blob-dir", filepath.Join(fix.blobs, "fb")}
			},
			wantText: wantText,
		},
		{
			name: "a directory holding the blob dir",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				elsewhere := filepath.Join(t.TempDir(), "m.json")

				return []string{"--out-blob-dir", dir, "--out", elsewhere}
			},
			wantText: wantText,
		},
		{
			name: "an existing unrelated directory is allowed",
			prepare: func(t *testing.T, fix fixture, dir string) []string {
				other := filepath.Join(dir, "other")
				if err := os.Mkdir(other, 0o750); err != nil {
					t.Fatal(err)
				}

				return []string{"--out-blob-dir", other}
			},
			wantText: "",
		},
	}

	runGuard(t, testCases)
}

func runGuard(t *testing.T, testCases []guardCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix := stage(t, []string{`{"position":1}`}, nil, fakeConfig)
			dir := filepath.Dir(fix.manifest)

			_, err := parse(fix.args(testCase.prepare(t, fix, dir)...))
			if testCase.wantText == "" {
				if err != nil {
					t.Fatalf("parse() err = %v, want none", err)
				}

				return
			}

			if !errors.Is(err, write.ErrUsage) ||
				!strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("parse() err = %v, want usage %q", err, testCase.wantText)
			}
		})
	}
}

func linkTo(t *testing.T, target, dir, name string) string {
	t.Helper()

	link := filepath.Join(dir, name)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	return link
}
