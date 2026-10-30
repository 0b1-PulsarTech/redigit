package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/resolve"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
	"github.com/wrapped-owls/redigit/internal/infra/lsp"
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

func buildFlags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}

	// Go keeps only the last -tags flag, so the tags travel in one.
	return []string{"-tags=" + strings.Join(tags, ",")}
}

func serverStarter(cfg Config, dir string) lsp.Starter {
	return func(ctx context.Context) (lsp.Backend, error) {
		session, err := lsp.StartSession(ctx, lsp.ServerConfig{
			Command:    cfg.ServerCommand,
			Args:       cfg.ServerArgs,
			RootPath:   dir,
			BuildFlags: buildFlags(cfg.BuildTags),
		})
		if err != nil {
			return nil, fmt.Errorf("start %s: %w", cfg.ServerCommand, err)
		}

		return session, nil
	}
}

const (
	ResolverLSP   = "lsp"
	ResolverTyped = "typed"
)

var ErrResolver = errors.New("unknown resolver")

type wiredResolver struct {
	resolver indexing.Resolver
	session  closer
	// Set only when a language server is started lazily and so can fail after the wiring.
	failed func() error
}

func wireResolver(
	ctx context.Context,
	cfg Config,
	snapshot entities.Snapshot,
	source *typed.Source,
	start lsp.Starter,
) (wiredResolver, error) {
	switch cfg.Resolver {
	case ResolverLSP:
		session, err := start(ctx)
		if err != nil {
			return wiredResolver{}, fmt.Errorf("start language server: %w", err)
		}

		return wiredResolver{resolver: session, session: session}, nil
	case "", ResolverTyped:
		// gopls costs about 40 s to start, so it waits for the first query typing cannot answer.
		// The decorator recognises a failed start by its own sentinel, so it never names a server.
		marked := func(ctx context.Context) (lsp.Backend, error) {
			session, err := start(ctx)
			if err != nil {
				return nil, fmt.Errorf("%w: %w", resolve.ErrSecondaryStart, err)
			}

			return session, nil
		}
		fallback := resolve.NewFallback(
			resolve.NewTyped(source, snapshot), lsp.NewLazySession(marked), slog.Default(),
		)

		return wiredResolver{resolver: fallback, session: fallback, failed: fallback.Err}, nil
	}

	return wiredResolver{}, fmt.Errorf("%w: %q", ErrResolver, cfg.Resolver)
}

func CheckResolver(name string) error {
	if name != ResolverLSP && name != ResolverTyped {
		return fmt.Errorf("%w: %q, want %s or %s", ErrResolver, name, ResolverLSP, ResolverTyped)
	}

	return nil
}

type guardedIndexer struct {
	inner  indexer
	failed func() error
}

func (wired wiredResolver) guard(inner indexer) guardedIndexer {
	return guardedIndexer{inner: inner, failed: wired.failed}
}

func (guard guardedIndexer) Index(
	ctx context.Context,
	snap entities.Snapshot,
) (indexing.Result, error) {
	result, err := guard.inner.Index(ctx, snap)

	// The indexer files a failed query as a blind spot and goes on, so a graph answered without
	// the server it needed would read as complete. It is refused here, before anything saves it.
	if guard.failed != nil {
		if failure := guard.failed(); failure != nil {
			return indexing.Result{}, errors.Join(fmt.Errorf(
				"typed resolver fell back to a server that failed: %w", failure,
			), err)
		}
	}

	return result, err //nolint:wrapcheck // Pipeline.Graph words it
}
