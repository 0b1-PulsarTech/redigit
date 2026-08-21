package planning

import (
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestIsDraft(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		subject string
		want    bool
	}{
		{"generated", DraftMarker + "feat(lead): Create", true},
		{"authored", ":seedling: feat(lead): Register a lead", false},
		{"empty", "", false},
		{"the marker only counts at the start", "feat: " + DraftMarker, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := IsDraft(testCase.subject); got != testCase.want {
				t.Fatalf("IsDraft(%q) = %v, want %v", testCase.subject, got, testCase.want)
			}
		})
	}
}

func TestDraftSubject(t *testing.T) {
	t.Parallel()

	keys := []entities.DeclKey{
		{File: "a.go", Kind: entities.KindType, Name: "Lead"},
		{File: "b.go", Kind: entities.KindFunc, Name: "Create"},
	}

	testCases := []struct {
		name     string
		shape    entities.CommitShape
		feature  FeatureID
		keys     []entities.DeclKey
		wantHas  []string
		wantKind string
	}{
		{
			"a feature commit is scoped to the last path segment",
			entities.ShapeFeature,
			"modules/lead/api",
			keys,
			[]string{"feat(api)", "Lead", "Create"},
			"feat",
		},
		{
			"a layer commit is not a feature and does not claim to be",
			entities.ShapeLayer,
			SharedFeature,
			keys,
			[]string{"chore(shared)"},
			"chore",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := draftSubject(testCase.shape, testCase.feature, testCase.keys)
			if !IsDraft(got) {
				t.Fatalf("subject %q is not marked as a draft", got)
			}

			for _, want := range testCase.wantHas {
				if !strings.Contains(got, want) {
					t.Fatalf("subject %q does not contain %q", got, want)
				}
			}
		})
	}
}

func TestDescribe(t *testing.T) {
	t.Parallel()

	key := func(name string) entities.DeclKey {
		return entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: name}
	}

	testCases := []struct {
		name string
		keys []entities.DeclKey
		want string
	}{
		{"empty", nil, ""},
		{"one", []entities.DeclKey{key("A")}, "A"},
		{"exactly the shown limit", []entities.DeclKey{key("A"), key("B"), key("C")}, "A, B, C"},
		{
			"past the limit the rest is counted, not truncated silently",
			[]entities.DeclKey{key("A"), key("B"), key("C"), key("D"), key("E")},
			"A, B, C and 2 more",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := describe(testCase.keys); got != testCase.want {
				t.Fatalf("describe() = %q, want %q", got, testCase.want)
			}
		})
	}
}
