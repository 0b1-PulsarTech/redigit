package inputs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestParseLayers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		path    string
		want    entities.Layer
	}{
		{
			name:    "a rule and a comment on the same line",
			content: "1 libs/tereckernel   # the kernel\n",
			path:    "libs/tereckernel/lead.go",
			want:    1,
		},
		{
			name:    "blank lines and whole-line comments are skipped",
			content: "\n# ordering\n\n  4   database  \n",
			path:    "database/queries/lead.sql",
			want:    4,
		},
		{
			name:    "a leading and trailing slash on the prefix is trimmed",
			content: "6 /libs/infra/\n",
			path:    "libs/infra/repo.go",
			want:    6,
		},
		{
			name:    "layer 0 is a real layer, not an absent one",
			content: "0 tools\n",
			path:    "tools/gen.go",
			want:    0,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			layers, err := ParseLayers([]byte(testCase.content))
			if err != nil {
				t.Fatalf("ParseLayers() = %v", err)
			}

			if got := layers.LayerOf(testCase.path); got != testCase.want {
				t.Fatalf("LayerOf(%q) = %d, want %d", testCase.path, got, testCase.want)
			}
		})
	}
}

// Every malformed line is refused rather than skipped: a dropped rule changes
// the order of the whole rebuild and nothing downstream would report it.
func TestParseLayersRefusesAMalformedLine(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
	}{
		{name: "only a prefix, no layer", content: "libs/tereckernel\n"},
		{name: "three fields", content: "1 libs/tereckernel extra\n"},
		{name: "the layer is not a number", content: "kernel libs/tereckernel\n"},
		{name: "a negative layer collides with NoLayer", content: "-1 libs/tereckernel\n"},
		{name: "the prefix is only slashes", content: "1 ///\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseLayers([]byte(testCase.content))
			if !errors.Is(err, ErrMalformedLayer) {
				t.Fatalf("ParseLayers() err = %v, want ErrMalformedLayer", err)
			}
		})
	}
}

func TestLoadLayersReadsAFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "layers.txt")
	if err := os.WriteFile(path, []byte("1 libs/tereckernel\n6 libs/infra\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	layers, err := LoadLayers(path)
	if err != nil {
		t.Fatalf("LoadLayers() = %v", err)
	}

	if got := layers.LayerOf("libs/infra/repo.go"); got != 6 {
		t.Fatalf("LayerOf = %d, want 6", got)
	}
}

func TestLoadLayersFailsOnAMissingFile(t *testing.T) {
	t.Parallel()

	_, err := LoadLayers(filepath.Join(t.TempDir(), "absent.txt"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadLayers() err = %v, want it to wrap os.ErrNotExist", err)
	}
}
