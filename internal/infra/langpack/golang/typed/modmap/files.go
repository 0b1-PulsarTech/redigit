package modmap

import (
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

type Files struct {
	root    string
	names   *sync.Map
	modules table
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

	return files
}

func (files Files) Rel(name string) string {
	if cached, known := files.names.Load(name); known {
		return cached.(string)
	}

	disk := name
	if !filepath.IsAbs(name) {
		disk = files.modules.disk(name)
	}

	// a miss race recomputes the same value, so a plain Store is enough
	slashed := files.under(disk)
	files.names.Store(name, slashed)

	return slashed
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
