package inputs

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
		"# slices\n\n0-12 bootstrap\n17,258,575  tags de lead  # far apart\n" +
			"243-245  agendamento\n",
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
		{name: "first position of a range", at: 0, want: "bootstrap"},
		{name: "last position of a range is inclusive", at: 12, want: "bootstrap"},
		{name: "a lone position of a discontinuous slice", at: 17, want: "tags de lead"},
		{name: "a distant position of the same slice", at: 575, want: "tags de lead"},
		{name: "a position between two points of a slice belongs to neither", at: 100, want: ""},
		{name: "inside a later range", at: 244, want: "agendamento"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := ""
			if index := features.IndexAt(testCase.at); index >= 0 {
				got = all[index].Name
			}

			if got != testCase.want {
				t.Fatalf("the slice holding %d is %q, want %q", testCase.at, got, testCase.want)
			}
		})
	}
}

// The whole reason the first field is a set: a capability the published
// history built in pieces hundreds of positions apart must not swallow what
// lies between them.
func TestParseFeaturesCoversOnlyThePositionsNamed(t *testing.T) {
	t.Parallel()

	features, err := ParseFeatures([]byte("17,258,575 tags de lead\n"))
	if err != nil {
		t.Fatalf("ParseFeatures() = %v", err)
	}

	if got := features.Covered(); got != 3 {
		t.Fatalf("the slice covers %d positions, want exactly the 3 named", got)
	}
}

// Two slices claiming one position make the grouping ambiguous, and IndexAt
// would silently answer with whichever sorted first.
func TestParseFeaturesRefusesAnOverlap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
	}{
		{name: "two ranges sharing a position", content: "0-100 first\n50-200 second\n"},
		{name: "a point inside another slice's range", content: "0-100 first\n50 second\n"},
		{name: "the same position twice", content: "7 first\n7 second\n"},
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

func TestParseFeaturesRefusesAMalformedLine(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
	}{
		{name: "no name", content: "0-12\n"},
		{name: "the position is not a number", content: "start boot\n"},
		{name: "a range end is not a number", content: "0-end boot\n"},
		{name: "a negative position", content: "-3 boot\n"},
		{name: "a range that ends before it starts", content: "12-0 boot\n"},
		{name: "an empty element in the set", content: "1,,3 boot\n"},
		{name: "a trailing comma", content: "1,2, boot\n"},
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

	features, err := ParseFeatures([]byte("0-5 agendamento de horarios\n"))
	if err != nil {
		t.Fatalf("ParseFeatures() = %v", err)
	}

	if got := features.All()[0].Name; got != "agendamento de horarios" {
		t.Fatalf("name = %q, want the whole rest of the line", got)
	}
}

// A position named twice inside ONE slice is the caller repeating itself, not
// two slices disagreeing, so it is deduplicated rather than refused.
func TestParseFeaturesDeduplicatesWithinOneSlice(t *testing.T) {
	t.Parallel()

	features, err := ParseFeatures([]byte("5,5,3-6 tags\n"))
	if err != nil {
		t.Fatalf("ParseFeatures() = %v", err)
	}

	if got := features.Covered(); got != 4 {
		t.Fatalf("covered %d positions, want 4 (3 through 6)", got)
	}
}

func TestLoadFeaturesReadsAFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "features.txt")
	if err := os.WriteFile(path, []byte("243-245 agendamento\n"), 0o600); err != nil {
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
