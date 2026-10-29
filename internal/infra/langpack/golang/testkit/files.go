package testkit

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	dirMode  = 0o750
	fileMode = 0o600
)

func WriteFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), dirMode); err != nil {
		t.Fatalf("mkdir for %s: %v", relPath, err)
	}

	if err := os.WriteFile(full, []byte(content), fileMode); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}
