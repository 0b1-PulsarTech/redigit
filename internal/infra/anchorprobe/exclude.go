package anchorprobe

import (
	"cmp"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func trialled(units []verifying.Unit, exclude, files []string) (kept, left []verifying.Unit) {
	kept = slices.DeleteFunc(slices.Clone(units), func(unit verifying.Unit) bool {
		return verifying.IsExcluded(unit.Dir, exclude)
	})

	for _, unit := range units {
		// A kept unit that requires an excluded one compiles only what it imports, so only the
		// excluded unit that owns an edited file is left out; one that merely depends on the
		// edit still has no file of its own in it.
		isOwner := slices.ContainsFunc(files, func(file string) bool {
			return owns(units, unit.Dir, file)
		})
		if verifying.IsExcluded(unit.Dir, exclude) && isOwner {
			left = append(left, unit)
		}
	}

	return kept, left
}

func dirsOf(units []verifying.Unit) string {
	dirs := make([]string, 0, len(units))
	for _, unit := range units {
		dirs = append(dirs, cmp.Or(unit.Dir, "."))
	}

	return strings.Join(dirs, ", ")
}

func uncheckedBy(
	verdict anchoring.Verdict,
	kept, left []verifying.Unit,
) anchoring.Verdict {
	if len(left) == 0 || !verdict.IsBuilt {
		return verdict
	}

	return anchoring.Verdict{
		IsUnchecked: true,
		Detail: "built " + dirsOf(
			kept,
		) + "; excluded by verify.exclude and not built: " + dirsOf(
			left,
		),
	}
}
