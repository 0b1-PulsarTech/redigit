package web

import (
	"context"
	"path"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// testAnchorRule pulls a test toward what it exercises: the first file outside
// test/ it imports, or else the first page it visits by path. A suite that
// only drives the built app imports nothing of it, so the path it requests is
// the only name it gives its subject.
type testAnchorRule struct{}

func (testAnchorRule) Name() string { return "test-anchor" }

func (testAnchorRule) Edges(context.Context, *index) ([]entities.Edge, error) { return nil, nil }

func (testAnchorRule) Anchors(ctx context.Context, run *index) ([]entities.Anchor, error) {
	var out []entities.Anchor

	for _, file := range run.scripts {
		for _, decl := range run.byFile[file] {
			if !decl.IsTest {
				continue
			}

			subject, found, err := run.subject(ctx, file)
			if err != nil {
				return nil, err
			}

			if to, isDeclared := run.first(subject); found && isDeclared {
				out = append(
					out,
					entities.Anchor{Decl: decl.Key, To: to, Reason: entities.ReasonTestOf},
				)
			}
		}
	}

	return out, nil
}

func (run *index) subject(ctx context.Context, test string) (string, bool, error) {
	targets, err := run.targets(ctx, test)
	if err != nil {
		return "", false, err
	}

	for _, target := range targets {
		if !isUnderTest(target) {
			return target, true, nil
		}
	}

	source, err := run.space.read(ctx, test)
	if err != nil {
		return "", false, err
	}

	if err = run.space.load(ctx); err != nil {
		return "", false, err
	}

	root, _, isPackaged := nearest(run.space.packages, path.Dir(test))
	if !isPackaged {
		return "", false, nil
	}

	for _, visited := range visits(string(source)) {
		if page, found := run.routeFor(root, visited); found {
			return page, true, nil
		}
	}

	return "", false, nil
}
