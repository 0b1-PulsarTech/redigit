package typed

import (
	"slices"

	"golang.org/x/tools/go/packages"
)

type Shortfall struct {
	Broken int
	Of     int
	Tags   []string
}

func (source *Source) noteBroken(pkgs []*packages.Package, tags []string) {
	if broken := brokenPackages(pkgs); broken > 0 {
		source.guard.Lock()
		defer source.guard.Unlock()

		source.short = append(source.short, Shortfall{Broken: broken, Of: len(pkgs), Tags: tags})
	}
}

func (source *Source) Shortfalls() []Shortfall {
	source.guard.Lock()
	defer source.guard.Unlock()

	return slices.Clone(source.short)
}
