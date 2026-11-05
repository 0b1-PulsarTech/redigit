package resume

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
)

type (
	flagBuilder func() *flag.FlagSet

	target struct {
		repo, rev, config, stateDir string
		end                         int
	}
)

func scan(build flagBuilder, args []string) (target, error) {
	set := build()
	if err := set.Parse(args); err != nil {
		return target{}, fmt.Errorf("%w: the stored command line: %w", anchorstate.ErrCorrupt, err)
	}

	// The parser takes the terminator off the arguments, but the config must go in before it.
	end := len(args) - set.NArg()
	if end > 0 && args[end-1] == "--" && endsAtTerminator(build, args[:end-1]) {
		end--
	}

	found := target{end: end}

	for name, dest := range map[string]*string{
		"repo": &found.repo, "rev": &found.rev, "config": &found.config, "state-dir": &found.stateDir,
	} {
		registered := set.Lookup(name)
		if registered == nil {
			return target{}, fmt.Errorf("%w: the flag set lacks --%s", anchorstate.ErrCorrupt, name)
		}

		*dest = registered.Value.String()
	}

	return found, nil
}

func endsAtTerminator(build flagBuilder, head []string) bool {
	probe := build()

	return probe.Parse(head) == nil && probe.NArg() == 0
}

func (found target) withConfig(args []string, derived string) []string {
	return slices.Insert(slices.Clone(args), 1+found.end, "--config", derived)
}

func (found target) isDerived(dir string) bool {
	derived := filepath.Join(dir, anchorstate.DerivedFile)
	left, _ := filepath.Abs(found.config)
	right, _ := filepath.Abs(derived)

	if left == right {
		return true
	}

	first, firstErr := os.Stat(found.config)
	second, secondErr := os.Stat(derived)

	return firstErr == nil && secondErr == nil && os.SameFile(first, second)
}
