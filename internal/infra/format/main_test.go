package format

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "redigit-fakefmt-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	build := exec.CommandContext(
		context.Background(),
		"go",
		"build",
		"-o",
		filepath.Join(dir, "fakefmt"),
		"./testdata/fakefmt",
	)
	build.Stderr = os.Stderr

	if err = build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build fakefmt:", err)
		os.RemoveAll(dir)
		os.Exit(2)
	}

	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	code := m.Run()

	os.RemoveAll(dir)
	os.Exit(code)
}
