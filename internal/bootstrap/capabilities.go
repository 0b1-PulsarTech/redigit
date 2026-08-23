package bootstrap

import (
	"context"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning"
)

// capabilitiesFrom maps each declaration to the slice holding the position it
// was born at. The birth is the declaration's own, not its file's: a file
// grown over months belongs to as many slices as it was grown in, and dating
// its declarations by the file misfiled 510 of 14.578 on a real repository.
func capabilitiesFrom(births indexing.Births, features entities.Features) planning.Capabilities {
	if features.Empty() {
		return nil
	}

	all := features.All()
	out := make(planning.Capabilities, len(births.At))

	for key, at := range births.At {
		// A declaration born where no slice reaches is left unnamed, which
		// the planner reads as no slice rather than as an empty one.
		index := features.IndexAt(at)
		if index < 0 {
			continue
		}

		out[key] = planning.FeatureID(all[index].Name)
	}

	return out
}

// loadCapabilities fills the planner's slicing when the caller supplied one.
// The per-declaration birth scan it costs is paid only then: the planner has
// to know which slice a declaration belongs to before it cuts, and only the
// scan answers that.
func loadCapabilities(
	ctx context.Context,
	cfg *Config,
	scanner indexing.Scanner,
	snapshot entities.Snapshot,
) error {
	if cfg.Ordering.Features.Empty() {
		return nil
	}

	births, err := scanner.Scan(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("scan births for slices: %w", err)
	}

	cfg.Planning.Capabilities = capabilitiesFrom(births, cfg.Ordering.Features)

	return nil
}
