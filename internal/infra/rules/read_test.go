package rules

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t testing.TB, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "redigit.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	return path
}

func TestReadBounded(t *testing.T) {
	t.Parallel()

	oversize := `{"rules":[],"slice":"` + strings.Repeat("x", maxConfigBytes) + `"}`
	atCap := `{"slice":"` + strings.Repeat("x", maxConfigBytes-len(`{"slice":""}`)) + `"}`

	testCases := []struct {
		name    string
		path    func(t testing.TB) string
		wantErr error
	}{
		{"a directory", func(t testing.TB) string { return t.TempDir() }, ErrNotRegular},
		{
			"a file past the cap",
			func(t testing.TB) string { return writeFile(t, oversize) },
			ErrTooLarge,
		},
		{
			"a file exactly at the cap",
			func(t testing.TB) string { return writeFile(t, atCap) }, nil,
		},
		{
			"a missing file",
			func(t testing.TB) string { return filepath.Join(t.TempDir(), "absent.json") },
			os.ErrNotExist,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := ReadBounded(testCase.path(t))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ReadBounded() err = %v, want %v", err, testCase.wantErr)
			}

			if err == nil && len(got) == 0 {
				t.Fatal("ReadBounded() returned no content for a file that fits")
			}
		})
	}
}
