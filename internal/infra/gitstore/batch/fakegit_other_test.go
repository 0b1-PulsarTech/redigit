//go:build !unix

package batch

import (
	"os"
	"testing"
)

func useFakeGit(t testing.TB, _, _ string) string {
	t.Helper()
	t.Skip("the fake git is a shell script behind named pipes, which needs a unix host")

	return ""
}

func isReaped(pid int) bool {
	_, err := os.FindProcess(pid)

	return err != nil
}
