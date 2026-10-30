package plancache

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string, mode os.FileMode) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
}

func TestFileDigest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	abc := writeFile(t, dir, "abc", "abc", 0o600)
	empty := writeFile(t, dir, "empty", "", 0o600)

	testCases := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			"known bytes", abc,
			"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", false,
		},
		{
			"empty file", empty,
			"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", false,
		},
		{"missing file", filepath.Join(dir, "absent"), "", true},
		{"a directory", dir, "", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := FileDigest(testCase.path)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("FileDigest() err = %v, wantErr %v", err, testCase.wantErr)
			}

			if got != testCase.want {
				t.Fatalf("FileDigest() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestExecutableDigest(t *testing.T) {
	t.Parallel()

	got, err := ExecutableDigest()
	if err != nil || len(got) != 64 {
		t.Fatalf("ExecutableDigest() = %q, %v, want a sha256 digest", got, err)
	}
}

func TestCommandDigest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	server := writeFile(t, dir, "server", "#!/bin/sh\n", 0o755)
	plain := writeFile(t, dir, "plain", "data", 0o600)

	testCases := []struct {
		name    string
		command string
		wantErr bool
	}{
		{"an executable path", server, false},
		{"a file that is not executable", plain, true},
		{"a command nothing provides", "redigit-no-such-server-binary", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := CommandDigest(testCase.command)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("CommandDigest() err = %v, wantErr %v", err, testCase.wantErr)
			}

			if !testCase.wantErr && len(got) != 64 {
				t.Fatalf("CommandDigest() = %q, want a sha256 digest", got)
			}
		})
	}
}
