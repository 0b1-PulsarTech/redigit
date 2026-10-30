package typed

import (
	"go/token"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestPositionsDeclAtConcurrent(t *testing.T) {
	root := t.TempDir()

	testCases := []struct {
		name    string
		files   []string
		workers int
	}{
		{name: "one file repeated", files: []string{"a.go"}, workers: 8},
		{name: "distinct files", files: []string{"a.go", "b.go", "c.go", "d.go"}, workers: 8},
		{name: "outside the root", files: []string{"../out.go", "a.go"}, workers: 8},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fset := token.NewFileSet()
			bases := make([]token.Pos, 0, len(testCase.files))

			for _, name := range testCase.files {
				file := fset.AddFile(filepath.Join(root, name), -1, 10)
				bases = append(bases, token.Pos(file.Base()))
			}

			shared := NewPositions(root, NewDeclLocator([]entities.Decl{}))
			copied := shared

			var group sync.WaitGroup

			for worker := range testCase.workers {
				where := shared
				if worker%2 == 1 {
					where = copied
				}

				group.Go(func() {
					for _, base := range bases {
						where.DeclAt(fset, base)
					}
				})
			}

			group.Wait()
		})
	}
}
