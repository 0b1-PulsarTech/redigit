package modmap

import (
	"bytes"
	"cmp"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

type Files struct {
	root    string
	names   *sync.Map
	modules table
	cgo     map[string]string
}

func New(root string) Files {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	return Files{root: root, names: &sync.Map{}}
}

func (files Files) Reading(pkgs []*packages.Package) Files {
	files.names = &sync.Map{}
	files.modules = tableOf(pkgs)
	files.cgo = cgoNames(pkgs)

	return files
}

func (files Files) Rel(name string) string {
	if cached, known := files.names.Load(name); known {
		return cached.(string)
	}

	// The build cache keeps the checkout that filled it in a cgo file's //line name.
	disk := cmp.Or(files.cgo[name], name)
	if !filepath.IsAbs(disk) {
		disk = files.modules.disk(disk)
	}

	// a miss race recomputes the same value, so a plain Store is enough
	slashed := files.under(disk)
	files.names.Store(name, slashed)

	return slashed
}

func (files Files) Offset(at token.Position) int {
	// A cgo file offset counts the checkout in its //line text, so count the listed file instead.
	listed, isCgo := files.cgo[at.Filename]
	if !isCgo {
		return at.Offset
	}

	source, err := os.ReadFile(listed) //nolint:gosec // a file the go tool listed
	if err != nil {
		return at.Offset
	}

	start := 0

	for range at.Line - 1 {
		next := bytes.IndexByte(source[start:], '\n')
		if next < 0 {
			return at.Offset
		}

		start += next + 1
	}

	return start + at.Column - 1
}

func (files Files) under(disk string) string {
	if disk == "" {
		return ""
	}

	resolved := disk
	if target, err := filepath.EvalSymlinks(disk); err == nil {
		resolved = target
	}

	rel, err := filepath.Rel(files.root, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = ""
	}

	return filepath.ToSlash(rel)
}

func cgoNames(pkgs []*packages.Package) map[string]string {
	names := map[string]string{}

	for _, pkg := range pkgs {
		if slices.Equal(pkg.GoFiles, pkg.CompiledGoFiles) {
			continue
		}

		for _, file := range pkg.Syntax {
			// A file the go tool listed was parsed where it lies: its //line names what the author wrote.
			if slices.Contains(pkg.GoFiles, pkg.Fset.File(file.FileStart).Name()) {
				continue
			}

			named := pkg.Fset.Position(file.Name.Pos()).Filename

			for _, listed := range pkg.GoFiles {
				if filepath.Base(listed) == filepath.Base(named) {
					names[named] = listed
				}
			}
		}
	}

	return names
}
