package testkit

import (
	"os"
	"path/filepath"
	"testing"
)

func WriteFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", relPath, err)
	}

	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}
