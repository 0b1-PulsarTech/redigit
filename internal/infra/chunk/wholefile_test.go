package chunk

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestWholeFileHandles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
	}{
		{"any path", "README.md"},
		{"empty path", ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := (WholeFile{}).Handles(testCase.path); !got {
				t.Fatalf("Handles(%q) = false, want true", testCase.path)
			}
		})
	}
}

func TestWholeFileChunks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		path   string
		source string
	}{
		{"non-empty file", "docs/README.md", "# Title\n\nBody.\n"},
		{"empty file", "docs/EMPTY.md", ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte(testCase.source)

			decls, err := (WholeFile{}).Chunks(context.Background(), testCase.path, source)
			if err != nil {
				t.Fatalf("Chunks() error = %v", err)
			}

			assertReconstructs(t, testCase.source, decls)

			if len(decls) != 1 {
				t.Fatalf("Chunks() = %d decls, want 1", len(decls))
			}

			decl := decls[0]
			if decl.Key != (entities.DeclKey{File: testCase.path, Kind: entities.KindOther, Name: fileDeclName}) {
				t.Fatalf("Key = %+v, want the whole-file sentinel key", decl.Key)
			}

			if decl.Unit != entities.UnitID(filepath.Dir(testCase.path)) {
				t.Fatalf("Unit = %q, want %q", decl.Unit, filepath.Dir(testCase.path))
			}
		})
	}
}

func TestWholeFileSplitsByLines(t *testing.T) {
	t.Parallel()

	lines := func(count int) string { return strings.Repeat("line\n", count) }

	testCases := []struct {
		name      string
		path      string
		source    string
		maxLines  int
		wantNames []string
	}{
		{
			"995 lines over 900",
			"docs/CHANGELOG.md",
			lines(995),
			900,
			[]string{"<file>", "<file>#2"},
		},
		{"exactly the cap stays whole", "docs/A.md", lines(900), 900, []string{"<file>"}},
		{"three runs", "docs/B.md", lines(7), 3, []string{"<file>", "<file>#2", "<file>#3"}},
		{"last line without newline", "docs/C.md", "a\nb\nc", 2, []string{"<file>", "<file>#2"}},
		{"no cap splits nothing", "docs/D.md", lines(2000), 0, []string{"<file>"}},
		{
			"generated stays whole",
			"docs/api.md",
			"# API (generated)\n" + lines(2000),
			900,
			[]string{"<file>"},
		},
		{"checksums stay whole", "go.sum", lines(2000), 900, []string{"<file>"}},
		{"binary stays whole", "assets/x.bin", "\x00" + lines(2000), 900, []string{"<file>"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls, err := (WholeFile{MaxLines: testCase.maxLines}).Chunks(
				context.Background(), testCase.path, []byte(testCase.source),
			)
			if err != nil {
				t.Fatalf("Chunks() error = %v", err)
			}

			assertReconstructs(t, testCase.source, decls)
			assertDeclNames(t, decls, testCase.wantNames)
		})
	}
}
