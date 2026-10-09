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

// subject searches the test, then the helpers under test/ it imports,
// breadth first: a suite runner names its cases, and only they name a page.
func (run *index) subject(ctx context.Context, test string) (string, bool, error) {
	queue, seen := []string{test}, map[string]bool{test: true}

	for len(queue) > 0 {
		file := queue[0]
		queue = queue[1:]

		found, helpers, err := run.ownSubject(ctx, file)
		if err != nil || found != "" {
			return found, found != "", err
		}

		for _, helper := range helpers {
			if !seen[helper] {
				seen[helper] = true

				queue = append(queue, helper)
			}
		}
	}

	return "", false, nil
}

// ownSubject is the first file outside test/ that file imports, or else the
// first page it visits, along with the files under test/ it imports.
func (run *index) ownSubject(ctx context.Context, file string) (string, []string, error) {
	targets, err := run.targets(ctx, file)
	if err != nil {
		return "", nil, err
	}

	var helpers []string

	for _, target := range targets {
		if !isUnderTest(target) {
			return target, nil, nil
		}

		helpers = append(helpers, target)
	}

	source, err := run.space.read(ctx, file)
	if err != nil {
		return "", nil, err
	}

	if err = run.space.load(ctx); err != nil {
		return "", nil, err
	}

	root, _, isPackaged := nearest(run.space.packages, path.Dir(file))
	if !isPackaged {
		return "", helpers, nil
	}

	for _, visited := range visits(string(source)) {
		if page, found := run.routeFor(root, visited); found {
			return page, nil, nil
		}
	}

	return "", helpers, nil
}
