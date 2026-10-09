package dart

import (
	"context"
	"fmt"
	"path"
	"strings"
)

type readFunc func(ctx context.Context, path string) ([]byte, error)

// workspace answers what a Dart URI points at and what each library of the
// tree declares. It parses each file once; Render and the rule pack both
// call it from one goroutine.
type workspace struct {
	files    map[string]bool
	read     readFunc
	packages map[string]string
	parsed   map[string]parsedFile
	exports  map[string]namespace
}

const pubspecName = "pubspec.yaml"

func newWorkspace(files []string, read readFunc) *workspace {
	set := make(map[string]bool, len(files))
	for _, file := range files {
		set[file] = true
	}

	return &workspace{
		files:   set,
		read:    read,
		parsed:  map[string]parsedFile{},
		exports: map[string]namespace{},
	}
}

// file parses path, or reports false for a path the tree does not hold.
func (w *workspace) file(ctx context.Context, file string) (parsedFile, bool, error) {
	if !w.files[file] {
		return parsedFile{}, false, nil
	}

	cached, found := w.parsed[file]

	if found {
		return cached, true, nil
	}

	source, err := w.read(ctx, file)
	if err != nil {
		return parsedFile{}, false, fmt.Errorf("read %s: %w", file, err)
	}

	parsed := parseFile(file, string(source))

	w.parsed[file] = parsed

	return parsed, true, nil
}

// resolve maps a URI written in from to a path of the tree, or to "" for a
// library outside it: the SDK, or a package no pubspec of the tree names.
func (w *workspace) resolve(ctx context.Context, from, uri string) (string, error) {
	var target string

	switch {
	case strings.HasPrefix(uri, "dart:"):
		return "", nil
	case strings.HasPrefix(uri, "package:"):
		name, rest, _ := strings.Cut(strings.TrimPrefix(uri, "package:"), "/")

		packages, err := w.packageRoots(ctx)
		if err != nil {
			return "", err
		}

		root, found := packages[name]
		if !found {
			return "", nil
		}

		target = path.Join(root, "lib", rest)
	default:
		target = path.Join(path.Dir(from), uri)
	}

	if !w.files[target] {
		return "", nil
	}

	return target, nil
}

// packageRoots maps each package name a pubspec.yaml declares to its
// directory, which is what a "package:" URI is resolved against.
func (w *workspace) packageRoots(ctx context.Context) (map[string]string, error) {
	known := w.packages

	if known != nil {
		return known, nil
	}

	roots := map[string]string{}

	for file := range w.files {
		if path.Base(file) != pubspecName {
			continue
		}

		source, err := w.read(ctx, file)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}

		if name := pubspecField(string(source), "name"); name != "" {
			roots[name] = path.Dir(file)
		}
	}

	w.packages = roots

	return roots, nil
}

// pubspecField reads a top-level scalar; pubspec.yaml keeps these unindented.
func pubspecField(source, field string) string {
	for line := range strings.Lines(source) {
		value, found := strings.CutPrefix(line, field+":")
		if found {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}

	return ""
}
