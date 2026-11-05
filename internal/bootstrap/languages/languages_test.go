package languages

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
	"github.com/wrapped-owls/redigit/internal/infra/sliceconf"
)

func TestSpecificLanguagesGivesOnlyTheGoPackTheSliceSection(t *testing.T) {
	t.Parallel()

	slice := sliceconf.Config{Functions: []string{"Run"}}
	languages := SpecificLanguages(config.Syntax{}, FinalTree{}, typed.NewSource(nil), slice)

	for _, language := range languages {
		selector, isSelector := language.Syntax.(indexing.Selector)
		if isSelector != (language.Name == GoLanguage) {
			t.Fatalf("%q selector = %v, want only %q", language.Name, isSelector, GoLanguage)
		}

		if isSelector && !slices.Equal(selector.Functions(), slice.Functions) {
			t.Fatalf("Functions() = %v, want %v", selector.Functions(), slice.Functions)
		}
	}
}
