package typed

import (
	"errors"
	"io/fs"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestModulesOf(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		written map[string]string
		listed  []string
		want    []module
		wantErr error
	}{
		{name: "no go.mod names no module", written: map[string]string{"a.go": "package a\n"}},
		{
			name:    "a root module",
			written: map[string]string{"go.mod": "module example.test/root\n"},
			listed:  []string{"go.mod", "a.go"},
			want:    []module{{dir: ".", path: "example.test/root"}},
		},
		{
			name: "nested modules keep their directories",
			written: map[string]string{
				"go.mod":       "module example.test/root\n",
				"tools/go.mod": "module example.test/tools\n",
			},
			listed: []string{"go.mod", "tools/go.mod"},
			want: []module{
				{dir: ".", path: "example.test/root"},
				{dir: "tools", path: "example.test/tools"},
			},
		},
		{
			name:    "a go.mod without a module line is skipped",
			written: map[string]string{"go.mod": "go 1.26\n"},
			listed:  []string{"go.mod"},
		},
		{
			name:    "a listed go.mod that cannot be read",
			written: map[string]string{},
			listed:  []string{"go.mod"},
			wantErr: fs.ErrNotExist,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			for name, content := range testCase.written {
				testkit.WriteFile(t, root, name, content)
			}

			got, err := modulesOf(entities.Snapshot{Root: root, Files: testCase.listed})
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("modulesOf() error = %v, want %v", err, testCase.wantErr)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("modulesOf() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestAllPatterns(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		modules []module
		want    []string
	}{
		{name: "no module has no pattern", want: []string{}},
		{
			name:    "one module is all its packages",
			modules: []module{{dir: ".", path: "example.test/root"}},
			want:    []string{"example.test/root/..."},
		},
		{
			name: "patterns come out sorted",
			modules: []module{
				{dir: "tools", path: "example.test/tools"},
				{dir: ".", path: "example.test/root"},
			},
			want: []string{"example.test/root/...", "example.test/tools/..."},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := allPatterns(testCase.modules); !slices.Equal(got, testCase.want) {
				t.Fatalf("allPatterns() = %v, want %v", got, testCase.want)
			}
		})
	}
}
