package chunk

import (
	"context"
	"path/filepath"
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
