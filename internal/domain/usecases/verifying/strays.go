package verifying

import (
	"fmt"
	"path"
	"strings"
)

func strayRow(base Row, strays, exclude []string) (Row, bool) {
	const (
		strayModule = "(outside any go.mod)"
		strayStep   = "module"
		strayShown  = 3
	)

	kept := make([]string, 0, len(strays))

	for _, file := range strays {
		if !isExcluded(path.Dir(file), exclude) {
			kept = append(kept, file)
		}
	}

	if len(kept) == 0 {
		return Row{}, false
	}

	tail := "go files outside any go.mod: " + strings.Join(kept[:min(len(kept), strayShown)], ", ")
	if len(kept) > strayShown {
		tail += fmt.Sprintf(" (and %d more)", len(kept)-strayShown)
	}

	base.Module, base.Language, base.Status = strayModule, "go", StatusFail
	base.Step, base.Tail = strayStep, tail

	return base, true
}
