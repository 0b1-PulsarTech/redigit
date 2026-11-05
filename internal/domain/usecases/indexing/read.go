package indexing

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
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

func (scanner Scanner) parseAll(
	ctx context.Context,
	units []blobAt,
	langs []Language,
) ([]parsedUnit, error) {
	parsed := make([]parsedUnit, len(units))

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(scanner.limit)

	for idx, unit := range units {
		group.Go(func() error {
			source, err := scanner.log.ReadBlob(groupCtx, unit.blob)
			if err != nil {
				if ctxErr := interrupted(
					groupCtx,
				); ctxErr != nil ||
					errors.Is(err, ErrBlobMissing) {
					return ctxErr
				}

				return fmt.Errorf("read blob: %w", err)
			}

			found, err := langs[idx].Syntax.Declarations(groupCtx, unit.path, source)
			if err != nil {
				return interrupted(groupCtx)
			}

			keys := make([]entities.DeclKey, 0, len(found))
			for _, decl := range found {
				keys = append(keys, decl.Key)
			}

			parsed[idx] = parsedUnit{keys: keys, isParsed: true}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, fmt.Errorf("scan births: %w", err)
	}

	return parsed, nil
}

func interrupted(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("interrupted: %w", err)
	}

	return nil
}

func (scanner Scanner) languageFor(file string) (Language, bool) {
	for _, lang := range scanner.langs {
		for _, ext := range lang.Extensions {
			if strings.HasSuffix(file, ext) {
				return lang, true
			}
		}
	}

	return Language{}, false
}
