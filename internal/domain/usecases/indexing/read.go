package indexing

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// read parses every owned file into declarations. Files are independent, so
// they are read concurrently; the locator is filled under a lock because a map
// is the one thing here that is not.
func (i Indexer) read(
	ctx context.Context,
	snap entities.Snapshot,
) ([]entities.Decl, locator, error) {
	var (
		guard sync.Mutex
		slots = make([][]entities.Decl, len(snap.Files))
		where = newLocator()
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(i.limit)

	for slot, file := range snap.Files {
		lang, owned := i.languageFor(file)
		if !owned {
			continue
		}

		group.Go(func() error {
			source, err := i.tree.Read(groupCtx, file)
			if err != nil {
				return fmt.Errorf("read %s: %w", file, err)
			}

			found, err := lang.Syntax.Declarations(groupCtx, file, source)
			if err != nil {
				return fmt.Errorf("read declarations of %s: %w", file, err)
			}

			measure(file, source, found)

			guard.Lock()
			defer guard.Unlock()

			slots[slot] = found
			where.add(file, source, found)

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, locator{}, fmt.Errorf("read declarations: %w", err)
	}

	decls := slices.Concat(slots...)
	slices.SortStableFunc(decls, compareDeclKeys)

	return decls, where, nil
}

// resolvedFile reports whether a language server answers for a path. A path no
// language owns is not asked about either.
func (i Indexer) resolvedFile(file string) bool {
	lang, owned := i.languageFor(file)

	return owned && lang.Resolved
}

func (i Indexer) languageFor(file string) (Language, bool) {
	for _, lang := range i.langs {
		if lang.Owns(file) {
			return lang, true
		}
	}

	return Language{}, false
}
