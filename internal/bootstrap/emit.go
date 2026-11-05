package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
	"github.com/wrapped-owls/redigit/internal/infra/atlassum"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
)

// treeSource adapts the git reader to what the emitter reads through: a file's
// final content together with the mode it must be written with. The two travel
// together because a mode that arrives separately eventually arrives wrong,
// and a script rebuilt as a plain file changes the tree without saying so.
type treeSource struct {
	reader gitstore.Reader
	modes  map[string]gitstore.FileMode
}

type readerCloser struct{ reader gitstore.Reader }

func (adapter readerCloser) Close(context.Context) error {
	if err := adapter.reader.Close(); err != nil {
		return fmt.Errorf("stop cat-file: %w", err)
	}

	return nil
}

type closers []closer

func (all closers) Close(ctx context.Context) error {
	var err error

	for _, one := range slices.Backward(all) {
		err = errors.Join(err, one.Close(ctx))
	}

	return err
}

// release closes what a failed start already opened. The start's own error is
// the one worth reporting, so a close error is dropped.
func release(ctx context.Context, opened ...closer) {
	for _, c := range opened {
		_ = c.Close(ctx)
	}
}

// Close stops what the hooks opened, the language server, the typed loads and the git reader, then
// removes the throwaway checkout, folding every failure so one does not hide
// another; the checkout is kept when the session or the loads fail to stop.
// It is registered by the caller right after Start, so a failure later still
// releases the process.
func (p *Pipeline) Close(ctx context.Context) error {
	// The session and typed loads read the checkout and stop first; one that fails keeps it.
	// The resolver goes before them: its index build reads the loads, so closing them under it
	// would log a skipped ad hoc pass that was only cancelled.
	hooksErr := p.hooked.Close(ctx)
	sessionErr := p.session.Close(ctx)
	loadsErr := p.loads.Close(ctx)
	closeErr := errors.Join(
		stopped("close hook resources", hooksErr),
		stopped("close language server", sessionErr),
		stopped("stop typed loads", loadsErr),
		stopped("close git reader", p.reader.Close(ctx)),
	)

	if keep := errors.Join(sessionErr, loadsErr); keep != nil {
		return errors.Join(closeErr, fmt.Errorf("keep checkout %s: %w", p.snapshot.Root, keep))
	}

	return errors.Join(closeErr, stopped("remove checkout", p.checkout.Close(ctx)))
}

func stopped(name string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	return nil
}

func newTreeSource(ctx context.Context, reader gitstore.Reader) (treeSource, error) {
	modes, err := reader.Modes(ctx)
	if err != nil {
		return treeSource{}, fmt.Errorf("read tree modes: %w", err)
	}

	return treeSource{reader: reader, modes: modes}, nil
}

func (s treeSource) Read(
	ctx context.Context,
	path string,
) ([]byte, emitting.FileMode, error) {
	content, err := s.reader.Read(ctx, path)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}

	mode, known := s.modes[path]
	if !known {
		return nil, "", fmt.Errorf("%w: %s carries no mode in the target tree", errNoMode, path)
	}

	return content, emitting.FileMode(mode), nil
}

// Emit materialises a reviewed plan. It writes blobs through sink and returns
// the commits; it does NOT write a commit, a ref or a history, because that
// judgement belongs to whoever consumes this.
func (p *Pipeline) Emit(
	ctx context.Context,
	plan entities.Plan,
	graph entities.Graph,
	sink emitting.Sink,
) ([]emitting.Commit, error) {
	commits, err := emitting.New(p.source, p.renderer, sink, atlassum.Deriver{}).
		Emit(ctx, plan, graph)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}

	return commits, nil
}

var errNoMode = errors.New("path has no mode in the target tree")

type importAdmitter interface {
	Admit(ctx context.Context, source []byte, specs []string) ([]byte, error)
}

func (dispatch byExtension) Admit(
	ctx context.Context,
	file string,
	source []byte,
	specs []string,
) ([]byte, error) {
	ext := path.Ext(file)

	admitter, isAdmitter := dispatch.byExt[ext].(importAdmitter)
	if !isAdmitter {
		return source, nil
	}

	out, err := admitter.Admit(ctx, source, specs)
	if err != nil {
		return nil, fmt.Errorf("admit into a %q file: %w", ext, err)
	}

	return out, nil
}
