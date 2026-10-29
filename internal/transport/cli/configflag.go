package cli

import (
	"flag"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
)

func applyConfigFile(cfg *bootstrap.Config, set *flag.FlagSet, configFile string) error {
	passed := map[string]bool{}

	set.Visit(func(visited *flag.Flag) { passed[visited.Name] = true })

	// An empty value given on purpose is a mistake, not "no config".
	if configFile == "" {
		if passed["config"] {
			return fmt.Errorf("%w: --config needs a path", bootstrap.ErrConfig)
		}

		return nil
	}

	file, err := bootstrap.LoadConfigFile(configFile)
	if err != nil {
		return err //nolint:wrapcheck // a config error names its file once; a second prefix would stutter
	}

	applied, err := bootstrap.ApplyConfigFile(
		*cfg, file, func(name string) bool { return passed[name] },
	)
	if err != nil {
		return err //nolint:wrapcheck // names its file once
	}

	*cfg = applied

	return nil
}
