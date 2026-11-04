package modmap

import (
	"path/filepath"
	"strings"
	"sync"
)

type Files struct {
	root  string
	names *sync.Map
}

func New(root string) Files {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	return Files{root: root, names: &sync.Map{}}
}

func (files Files) Rel(name string) string {
	if cached, known := files.names.Load(name); known {
		return cached.(string)
	}

	resolved := name
	if target, err := filepath.EvalSymlinks(name); err == nil {
		resolved = target
	}

	rel, err := filepath.Rel(files.root, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = ""
	}

	// a miss race recomputes the same value, so a plain Store is enough
	slashed := filepath.ToSlash(rel)
	files.names.Store(name, slashed)

	return slashed
}
