package selector

import (
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func importOf(spec string) (local, target string) {
	alias, rest, _ := strings.Cut(spec, `"`)
	target, _, _ = strings.Cut(rest, `"`)

	local = strings.TrimSpace(alias)
	if local == "" {
		local = path.Base(target)
	}

	if local == "_" || local == "." {
		local = ""
	}

	return local, target
}

func locals(imports []string) []string {
	var out []string

	for _, spec := range imports {
		if local, _ := importOf(spec); local != "" {
			out = append(out, local)
		}
	}

	return out
}

func Names(
	imports []string,
	unit entities.UnitID,
	modules map[string]bool,
) (names []string, isMatched bool) {
	base := path.Base(unit.Dir())

	var named []string

	all := make([]string, 0, len(imports))

	for _, spec := range imports {
		local, target := importOf(spec)
		all = append(all, local)

		if path.Base(target) == base {
			named = append(named, local)
			isMatched = true
		}
	}

	switch {
	case modules[unit.Dir()] || base == ".":
		// Only a module root can import under a base its directory lacks, so every import may be it.
		names, isMatched = all, len(imports) > 0
	case isMatched:
		names = append(named, base)
	}

	names = slices.DeleteFunc(slices.Sorted(slices.Values(names)), func(name string) bool {
		return name == ""
	})

	return slices.Compact(names), isMatched
}
