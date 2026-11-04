package typed

import (
	"cmp"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Shortfall struct {
	Broken       int
	Of           int
	Tags         []string
	FirstPackage string
	FirstError   string
	SameMessage  int
}

func (source *Source) noteBroken(pkgs []*packages.Package, tags []string) {
	short := shortfallOf(pkgs)
	if short.Broken == 0 {
		return
	}

	short.Of, short.Tags = len(pkgs), tags

	source.guard.Lock()
	defer source.guard.Unlock()

	source.short = append(source.short, short)
}

func (source *Source) Shortfalls() []Shortfall {
	source.guard.Lock()
	defer source.guard.Unlock()

	return slices.Clone(source.short)
}

func shortfallOf(pkgs []*packages.Package) Shortfall {
	var broken []*packages.Package

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			broken = append(broken, pkg)
		}
	}

	if len(broken) == 0 {
		return Shortfall{}
	}

	first := slices.MinFunc(broken, func(left, right *packages.Package) int {
		return cmp.Or(
			cmp.Compare(rankOf(left), rankOf(right)),
			cmp.Compare(left.PkgPath, right.PkgPath),
			cmp.Compare(left.ID, right.ID),
		)
	})
	cause := causeOf(first)
	shared := 0

	for _, pkg := range broken {
		if causeOf(pkg) == cause {
			shared++
		}
	}

	return Shortfall{
		Broken:       len(broken),
		FirstPackage: first.PkgPath,
		FirstError:   cause,
		SameMessage:  shared,
	}
}

func rankOf(pkg *packages.Package) int {
	const (
		pseudo = iota
		cause
		symptom
	)

	// The pseudo-package holds the real error; a package that could not import only echoes it.
	switch {
	case strings.HasSuffix(pkg.PkgPath, "/..."):
		return pseudo
	case !strings.HasPrefix(causeOf(pkg), "could not import"):
		return cause
	}

	return symptom
}

func oneLine(message string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(message), "\n")

	return strings.TrimSpace(line)
}

func causeOf(pkg *packages.Package) string {
	// A compile failure opens with a "# path" banner that names no cause.
	for _, found := range pkg.Errors {
		if line := oneLine(found.Msg); !strings.HasPrefix(line, "# ") {
			return line
		}
	}

	return oneLine(pkg.Errors[0].Msg)
}
