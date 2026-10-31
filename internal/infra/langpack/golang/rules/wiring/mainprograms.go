package wiring

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/buildtags"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

type programs struct {
	ignored map[string]bool
	plain   map[string]bool
}

func hostMains(
	decls []entities.Decl,
	inMain map[string]bool,
	progs programs,
) (byFile, byDir map[string][]entities.DeclKey) {
	byFile = map[string][]entities.DeclKey{}
	byDir = map[string][]entities.DeclKey{}

	for _, decl := range decls {
		file := decl.Key.File
		if !isMainFunc(decl.Key) || !inMain[file] {
			continue
		}

		byFile[file] = append(byFile[file], decl.Key)

		// A go:build ignore main is a separate program, not one a shared file waits for.
		if !progs.ignored[file] {
			byDir[path.Dir(file)] = append(byDir[path.Dir(file)], decl.Key)
		}
	}

	return byFile, byDir
}

func unconstrained(mains []entities.DeclKey, plain map[string]bool) []entities.DeclKey {
	always := slices.DeleteFunc(slices.Clone(mains), func(key entities.DeclKey) bool {
		return !plain[key.File]
	})
	if len(always) > 0 {
		return always
	}

	return mains
}

func programFiles(snap entities.Snapshot, inMain map[string]bool) (programs, error) {
	files := make([]string, 0, len(inMain))

	for file, isMain := range inMain {
		if isMain {
			files = append(files, file)
		}
	}

	slices.Sort(files)

	snap.Files = files

	constrained, err := buildtags.BuildConstraints(snap)
	if err != nil {
		return programs{}, fmt.Errorf("main package constraints: %w", err)
	}

	progs := programs{ignored: map[string]bool{}, plain: make(map[string]bool, len(files))}

	for _, file := range files {
		source, readErr := declsource.ReadSource(snap, file)
		if readErr != nil {
			return programs{}, fmt.Errorf("main package source: %w", readErr)
		}

		progs.plain[file] = buildsEverywhere(file, source)
	}

	for _, tag := range constrained {
		progs.plain[tag.File] = false
		progs.ignored[tag.File] = tag.IsIgnore
	}

	return progs, nil
}

func buildsEverywhere(file string, source []byte) bool {
	if importsC(file, source) {
		return false
	}

	// Two platforms that share nothing tell a GOOS or GOARCH file name suffix apart.
	for _, target := range [][2]string{{"linux", "amd64"}, {"windows", "arm64"}} {
		platform := build.Context{
			GOOS: target[0], GOARCH: target[1],
			OpenFile: func(string) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(source)), nil
			},
		}

		if built, err := platform.MatchFile(path.Dir(file), path.Base(file)); err != nil || !built {
			return false
		}
	}

	return true
}

func importsC(file string, source []byte) bool {
	parsed, err := parser.ParseFile(token.NewFileSet(), file, source, parser.ImportsOnly)
	if err != nil {
		return false
	}

	// Cgo is off on some targets, so a file that needs it does not build everywhere.
	return slices.ContainsFunc(parsed.Imports, func(spec *ast.ImportSpec) bool {
		return spec.Path.Value == `"C"`
	})
}

func isIgnoredName(name string) bool {
	return strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")
}
