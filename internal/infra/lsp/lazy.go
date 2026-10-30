package lsp

import (
	"context"
	"errors"
	"fmt"
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
