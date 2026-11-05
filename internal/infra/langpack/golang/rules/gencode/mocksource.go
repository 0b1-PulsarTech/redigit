package gencode

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type mockTarget struct {
	file string
	dir  string
}

func resolveMockSource(snap entities.Snapshot, mockDir, sourceLine string) (mockTarget, error) {
	path, ok := importPathSource(sourceLine)
	if !ok {
		return mockTarget{file: filepath.Join(mockDir, sourceLine)}, nil
	}

	dir, found, err := repoDirForImport(snap, path)
	if err != nil {
		return mockTarget{}, err
	}

	if !found {
		return mockTarget{}, nil
	}

	return mockTarget{dir: dir}, nil
}

func importPathSource(sourceLine string) (string, bool) {
	path, _, ok := strings.Cut(sourceLine, " (interfaces:")

	return strings.TrimSpace(path), ok
}

func repoDirForImport(snap entities.Snapshot, importPath string) (string, bool, error) {
	modPath, err := modulePath(snap)
	if err != nil {
		return "", false, err
	}

	if modPath == "" {
		return "", false, nil
	}

	if importPath == modPath {
		return ".", true, nil
	}

	rel, ok := strings.CutPrefix(importPath, modPath+"/")

	return rel, ok, nil
}

func modulePath(snap entities.Snapshot) (string, error) {
	data, err := os.ReadFile(filepath.Join(snap.Root, "go.mod"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}

	for line := range strings.Lines(string(data)) {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}

	return "", nil
}
