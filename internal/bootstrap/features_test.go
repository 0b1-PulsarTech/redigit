package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestParseFeatures(t *testing.T) {
	t.Parallel()

	features, err := ParseFeatures([]byte(
		"# slices\n\n0 12 bootstrap\n243  245  agendamento  # engine and module\n",
	))
	if err != nil {
		t.Fatalf("ParseFeatures() = %v", err)
	}

	all := features.All()

	testCases := []struct {
		name string
		at   entities.Position
		want string
	}{
		{name: "first position of the first slice", at: 0, want: "bootstrap"},
		{name: "last position of the first slice", at: 12, want: "bootstrap"},
		{name: "first position of the later slice", at: 243, want: "agendamento"},
		{name: "inside the later slice", at: 244, want: "agendamento"},
		{name: "a position between two slices belongs to neither", at: 100, want: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			index := features.IndexAt(testCase.at)

			got := ""
			if index >= 0 {
				got = all[index].Name
			}

			if got != testCase.want {
				t.Fatalf("the slice holding %d is %q, want %q", testCase.at, got, testCase.want)
			}
		})
	}
}

// Two slices claiming one position make the grouping ambiguous, and IndexAt
// would silently answer with whichever sorted first.
func TestParseFeaturesRefusesAnOverlap(t *testing.T) {
	t.Parallel()

	_, err := ParseFeatures([]byte("0 100 first\n50 200 second\n"))
	if !errors.Is(err, ErrMalformedFeature) {
		t.Fatalf("ParseFeatures() err = %v, want ErrMalformedFeature", err)
	}
}

func TestParseFeaturesRefusesAMalformedLine(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
	}{
		{name: "no name", content: "0 12\n"},
		{name: "first position is not a number", content: "start 12 boot\n"},
		{name: "last position is not a number", content: "0 end boot\n"},
		{name: "a negative position", content: "-3 12 boot\n"},
		{name: "last position before first", content: "12 0 boot\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseFeatures([]byte(testCase.content))
			if !errors.Is(err, ErrMalformedFeature) {
				t.Fatalf("ParseFeatures() err = %v, want ErrMalformedFeature", err)
			}
		})
	}
}

// A name may hold spaces: a feature is named by a person, not by an
// identifier, and truncating at the first space silently renames it.
func TestParseFeaturesKeepsAMultiWordName(t *testing.T) {
	t.Parallel()

	features, err := ParseFeatures([]byte("0 5 agendamento de horarios\n"))
	if err != nil {
		t.Fatalf("ParseFeatures() = %v", err)
	}

	if got := features.All()[0].Name; got != "agendamento de horarios" {
		t.Fatalf("name = %q, want the whole rest of the line", got)
	}
}

func TestLoadFeaturesReadsAFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "features.txt")
	if err := os.WriteFile(path, []byte("243 245 agendamento\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	features, err := LoadFeatures(path)
	if err != nil {
		t.Fatalf("LoadFeatures() = %v", err)
	}

	if features.IndexAt(244) != 0 {
		t.Fatal("the slice does not hold position 244")
	}
}

func TestLoadFeaturesFailsOnAMissingFile(t *testing.T) {
	t.Parallel()

	_, err := LoadFeatures(filepath.Join(t.TempDir(), "absent.txt"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LoadFeatures() err = %v, want it to wrap os.ErrNotExist", err)
	}
}
