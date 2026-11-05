package lsp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

//go:generate go tool -modfile=../../../tools/go.mod mockgen -source=lazy.go -destination=lazy_mock_test.go -package=lsp

var (
	ErrStart  = errors.New("start language server")
	ErrClosed = errors.New("language server closed")
)

type (
	Backend interface {
		indexing.Resolver
		Close(ctx context.Context) error
	}

	Starter func(ctx context.Context) (Backend, error)

	LazySession struct {
		start Starter

		guard   sync.Mutex
		tried   bool
		closed  bool
		session Backend
		err     error
	}
)

var _ indexing.Resolver = (*LazySession)(nil)

func NewLazySession(start Starter) *LazySession { return &LazySession{start: start} }

func (lazy *LazySession) References(
	ctx context.Context,
	at entities.Location,
) ([]entities.Location, error) {
	if err := lazy.ensure(ctx); err != nil {
		return nil, err
	}

	return lazy.session.References(ctx, at) //nolint:wrapcheck // the session words its own errors
}

func (lazy *LazySession) Implementations(
	ctx context.Context,
	at entities.Location,
) ([]entities.Location, error) {
	if err := lazy.ensure(ctx); err != nil {
		return nil, err
	}

	return lazy.session.Implementations(ctx, at) //nolint:wrapcheck // as References
}

func (lazy *LazySession) Definition(
	ctx context.Context,
	at entities.Location,
) (entities.Location, error) {
	if err := lazy.ensure(ctx); err != nil {
		return entities.Location{}, err
	}

	return lazy.session.Definition(ctx, at) //nolint:wrapcheck // as References
}

func (lazy *LazySession) Close(ctx context.Context) error {
	lazy.guard.Lock()
	defer lazy.guard.Unlock()

	lazy.closed = true

	if lazy.session == nil {
		return nil
	}

	return lazy.session.Close(ctx) //nolint:wrapcheck // as References
}

func (lazy *LazySession) ensure(ctx context.Context) error {
	lazy.guard.Lock()
	defer lazy.guard.Unlock()

	// A server started after Close would have no owner left to stop it.
	if lazy.closed {
		return ErrClosed
	}

	// The start can take tens of seconds; holding the lock is what makes later callers wait for
	// it instead of starting another server.
	if !lazy.tried {
		lazy.tried = true

		session, err := lazy.start(ctx)
		if err != nil {
			lazy.err = fmt.Errorf("%w: %w", ErrStart, err)
		} else {
			lazy.session = session
		}
	}

	return lazy.err
}

// ServerConfig configures which language server Session starts and how, so
// this package never hardcodes what "Go" means.
type ServerConfig struct {
	// Command and Args launch the server, e.g. "gopls", []string{"-mode=stdio"}.
	Command string
	Args    []string
	// RootPath is the workspace root directory. It is a plain path, not a
	// URI: encoding one is this package's job, not a caller's, and a caller
	// that has to build "file://..." by hand will eventually get a space or a
	// drive letter wrong.
	RootPath string
	// BuildFlags is REQUIRED and explicit on purpose: gopls answers
	// references/implementation/workspace-symbol only for files visible
	// under these flags, silently and without error otherwise. Passing
	// []string{"-tags=integration"} is what makes a file behind
	// //go:build integration visible to those queries; passing nil is a
	// conscious choice to exclude them, not an oversight this type lets
	// happen by accident.
	BuildFlags []string
	// InitializationOptions are extra flat gopls settings merged with
	// BuildFlags under the "buildFlags" key. gopls flattens nested JSON on
	// the last "."-separated segment, so this map must already be flat
	// (e.g. "buildFlags", not "build.buildFlags").
	InitializationOptions map[string]any
	// Logger receives debug-level transport diagnostics. Defaults to
	// slog.Default() when nil.
	Logger *slog.Logger
}

func (c ServerConfig) mergedInitOptions() map[string]any {
	merged := make(map[string]any, len(c.InitializationOptions)+1)
	maps.Copy(merged, c.InitializationOptions)
	merged["buildFlags"] = c.BuildFlags

	return merged
}
