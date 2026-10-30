package format

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCopyFiles(t *testing.T) {
	testCases := []struct {
		name    string
		names   func(t *testing.T, root string) []string
		wantErr bool
		isCfg   bool
		wantLen int
	}{
		{
			name:    "a file under the root is kept",
			names:   func(*testing.T, string) []string { return []string{"go.mod"} },
			wantLen: 1,
		},
		{
			name:    "a missing file is skipped",
			names:   func(*testing.T, string) []string { return []string{"nope.yml"} },
			wantLen: 0,
		},
		{
			name:    "a parent traversal is refused",
			names:   func(*testing.T, string) []string { return []string{"../../x"} },
			wantErr: true,
			isCfg:   true,
		},
		{
			name: "an absolute path is refused",
			names: func(_ *testing.T, root string) []string {
				return []string{filepath.Join(root, "go.mod")}
			},
			wantErr: true,
			isCfg:   true,
		},
		{
			name: "a symlink out of the root is refused",
			names: func(t *testing.T, root string) []string {
				t.Helper()

				outside := filepath.Join(t.TempDir(), "secret")
				if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
					t.Fatal(err)
				}

				if err := os.Symlink(outside, filepath.Join(root, "link.yml")); err != nil {
					t.Skipf("no symlinks here: %v", err)
				}

				return []string{"link.yml"}
			},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(root, "go.mod"),
				[]byte("module x\n"),
				0o600,
			); err != nil {
				t.Fatal(err)
			}

			config := fakeConfig(ModeFiles, 0)
			config.ConfigRoot, config.CopyFiles = root, testCase.names(t, root)

			engine, err := New(config)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("error = %v, want error %v", err, testCase.wantErr)
			}

			if testCase.isCfg && !errors.Is(err, ErrConfig) {
				t.Errorf("error = %v, want ErrConfig", err)
			}

			if err == nil && len(engine.configs) != testCase.wantLen {
				t.Errorf("configs = %d, want %d", len(engine.configs), testCase.wantLen)
			}
		})
	}
}

func TestWorkdirCopiesConfig(t *testing.T) {
	testCases := []formatCase{
		{
			name: "config files are copied beside the blobs",
			mode: ModeFiles,
			prepare: func(t *testing.T, config *Config) {
				t.Helper()

				config.ConfigRoot, config.CopyFiles = t.TempDir(), []string{"go.mod", "missing.yml"}

				err := os.WriteFile(
					filepath.Join(config.ConfigRoot, "go.mod"),
					[]byte("module x\n"),
					0o600,
				)
				if err != nil {
					t.Fatal(err)
				}
			},
			files: []File{{"a.go", []byte("a")}},
			want:  []string{"A"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, testCase.run)
	}
}
