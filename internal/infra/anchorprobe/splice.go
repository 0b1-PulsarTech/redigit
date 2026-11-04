package anchorprobe

import (
	"errors"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

var (
	ErrEdit  = errors.New("edit does not fit the file")
	ErrUnfit = errors.New("pre-image cannot be rendered")
)

func affectedBy(units []verifying.Unit, files []string) []verifying.Unit {
	var out []verifying.Unit

	for _, unit := range units {
		dirs := []string{unit.Dir}
		for _, dep := range unit.Deps {
			dirs = append(dirs, dep.Dir)
		}

		if slices.ContainsFunc(files, func(file string) bool {
			return slices.ContainsFunc(
				dirs,
				func(dir string) bool { return owns(units, dir, file) },
			)
		}) {
			out = append(out, unit)
		}
	}

	return out
}

func owns(units []verifying.Unit, dir, file string) bool {
	if !within(dir, file) {
		return false
	}

	// A nested module owns its files: the module around it never builds them.
	return !slices.ContainsFunc(units, func(other verifying.Unit) bool {
		return depth(other.Dir) > depth(dir) && within(other.Dir, file)
	})
}

func depth(dir string) int {
	if dir == "" || dir == "." {
		return 0
	}

	return strings.Count(strings.TrimSuffix(dir, "/"), "/") + 1
}

func within(dir, file string) bool {
	return dir == "" || dir == "." || strings.HasPrefix(file, strings.TrimSuffix(dir, "/")+"/")
}
