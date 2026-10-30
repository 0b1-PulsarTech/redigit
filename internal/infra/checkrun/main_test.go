package checkrun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const fakeGoDirEnv = "REDIGIT_FAKEGO_DIR"

func TestMain(m *testing.M) {
	os.Exit(runWithFakes(m))
}

func runWithFakes(m *testing.M) int {
	dir, err := os.MkdirTemp("", "redigit-fakecheck-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 2
	}

	defer os.RemoveAll(dir)

	goDir := filepath.Join(dir, "fakego")

	for target, source := range map[string]string{
		filepath.Join(dir, "fakecheck"): "./testdata/fakecheck",
		filepath.Join(goDir, "go"):      "./testdata/fakego",
	} {
		build := exec.CommandContext(context.Background(), "go", "build", "-o", target, source)
		build.Stderr = os.Stderr

		if err = build.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "build", source+":", err)

			return 2
		}
	}

	os.Setenv(fakeGoDirEnv, goDir)
	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return m.Run()
}
