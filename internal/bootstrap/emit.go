package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
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
	commits, err := emitting.New(p.source, p.renderer, sink).Emit(ctx, plan, graph)
	if err != nil {
		return nil, fmt.Errorf("emit: %w", err)
	}

	return commits, nil
}

var errNoMode = errors.New("path has no mode in the target tree")
