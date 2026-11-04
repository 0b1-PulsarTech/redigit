package modmap

import (
	"io/fs"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

type entry struct {
	dir     string
	isClash bool
}

type table struct {
	entries map[string]entry
}

func tableOf(pkgs []*packages.Package) table {
	found := table{entries: map[string]entry{}}

	packages.Visit(pkgs, nil, found.add)

	return found
}

func (found *table) add(pkg *packages.Package) {
	module := pkg.Module
	if module == nil {
		return
	}

	key := module.Path
	if module.Version != "" {
		key += "@" + module.Version
	}

	dir := module.Dir
	if dir == "" {
		dir = vendoredDir(pkg)
	}

	if resolved, err := filepath.EvalSymlinks(dir); err == nil && dir != "" {
		dir = resolved
	}

	known, seen := found.entries[key]
	if seen && (dir == "" || known.dir == dir) {
		return
	}

	if seen && known.dir != "" {
		found.entries[key] = entry{isClash: true}

		return
	}

	if !known.isClash {
		found.entries[key] = entry{dir: dir}
	}
}

func vendoredDir(pkg *packages.Package) string {
	if len(pkg.GoFiles) == 0 {
		return ""
	}

	inside := filepath.FromSlash(strings.TrimPrefix(pkg.PkgPath, pkg.Module.Path))
	dir := filepath.Dir(pkg.GoFiles[0])

	if !strings.HasSuffix(dir, inside) {
		return ""
	}

	return strings.TrimSuffix(dir, inside)
}

func (found table) disk(name string) string {
	name = filepath.ToSlash(name)
	owner := ""

	for key := range found.entries {
		if strings.HasPrefix(name, key+"/") && len(key) > len(owner) {
			owner = key
		}
	}

	rest := strings.TrimPrefix(name, owner+"/")
	if owner == "" || found.entries[owner].isClash || !fs.ValidPath(rest) {
		return ""
	}

	dir := found.entries[owner].dir
	if dir == "" {
		return ""
	}

	return filepath.Join(dir, filepath.FromSlash(rest))
}
