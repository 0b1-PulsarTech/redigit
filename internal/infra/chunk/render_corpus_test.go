package chunk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Rendering EVERY piece of a real go.mod must reproduce it byte for byte, or
// the rebuilt tree diverges and the invariant reports it only at the very end.
// A fixture cannot answer this: the shapes that break a formatter are the ones
// a real repository happens to have. Set REDIGIT_TREE to a checkout.
func TestRenderReproducesEveryRealModuleFile(t *testing.T) {
	t.Parallel()

	root := os.Getenv("REDIGIT_TREE")
	if root == "" {
		t.Skip("REDIGIT_TREE is unset")
	}

	var checked, diverged int

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}

		if entry.IsDir() {
			return nil
		}

		base := filepath.Base(path)
		if base != goModFileName && base != goWorkFileName {
			return nil
		}

		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}

		rel := strings.TrimPrefix(path, root+"/")

		var chunker interface {
			Chunks(ctx context.Context, path string, source []byte) ([]entities.Decl, error)
			Render(source []byte, keep []entities.DeclKey) ([]byte, error)
		} = GoMod{}

		if base == goWorkFileName {
			chunker = GoWork{}
		}

		decls, chunkErr := chunker.Chunks(t.Context(), rel, source)
		if chunkErr != nil {
			return fmt.Errorf("chunk %s: %w", rel, chunkErr)
		}

		all := make([]entities.DeclKey, 0, len(decls))
		for _, decl := range decls {
			all = append(all, decl.Key)
		}

		got, renderErr := chunker.Render(source, all)
		if renderErr != nil {
			return fmt.Errorf("render %s: %w", rel, renderErr)
		}

		checked++

		if string(got) != string(source) {
			diverged++

			if diverged <= 3 {
				t.Errorf("%s does not round trip:\ngot\n%s\nwant\n%s", rel, got, source)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if checked == 0 {
		t.Fatal("no go.mod or go.work was checked: the run proves nothing")
	}

	t.Logf("checked %d module files, %d diverged", checked, diverged)
}
