package indexing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Births is where each declaration first appeared in an existing history, and
// what the scan could not read.
type Births struct {
	At map[entities.DeclKey]entities.Position
	// Unreadable names the (revision, path) pairs the scan skipped. It is
	// reported rather than swallowed: a scan that silently skipped a tenth of
	// the history would still return a plausible-looking map.
	Unreadable []string
}

// Scanner finds, per declaration, the first commit whose version of its file
// already held it.
//
// This is the answer the file's first touch only approximates. A file is born
// with three declarations and gains eight over months, so anchoring on the
// file dates all of them to the earliest, and pairing a test with its twin
// file's birth rather than with the declaration it exercises is what breaks a
// move: the twin was born nearly empty and grew hundreds of commits later.
type Scanner struct {
	tree  RevisionTree
	langs []Language
	limit int
}

func NewScanner(tree RevisionTree, cfg Config) Scanner {
	limit := cfg.Concurrency
	if limit < 1 {
		limit = defaultConcurrency
	}

	return Scanner{tree: tree, langs: cfg.Languages, limit: limit}
}

// Scan walks every commit in the snapshot's history, reads each file that
// commit touched as it stood then, and records the earliest position each
// declaration key appears at.
func (s Scanner) Scan(ctx context.Context, snap entities.Snapshot) (Births, error) {
	if !snap.HasHistory() {
		return Births{}, fmt.Errorf("%w: the snapshot carries no history", errNoHistory)
	}

	var (
		guard      sync.Mutex
		at         = map[entities.DeclKey]entities.Position{}
		unreadable []string
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.limit)

	for _, touch := range snap.History {
		for _, path := range touch.Touched {
			lang, owned := s.languageFor(path)
			if !owned {
				continue
			}

			group.Go(func() error {
				found, err := s.declsAt(groupCtx, lang, touch.Rev, path)
				if err != nil {
					guard.Lock()
					unreadable = append(unreadable, touch.Rev+":"+path)
					guard.Unlock()

					return nil
				}

				guard.Lock()
				defer guard.Unlock()

				for _, decl := range found {
					if current, seen := at[decl.Key]; !seen || touch.Position < current {
						at[decl.Key] = touch.Position
					}
				}

				return nil
			})
		}
	}

	if err := group.Wait(); err != nil {
		return Births{}, fmt.Errorf("scan births: %w", err)
	}

	return Births{At: at, Unreadable: unreadable}, nil
}

var errNoHistory = errors.New("no source history to scan")

// declsAt parses one file as it stood at one revision. A file the revision
// does not hold, or one that does not parse as it stood then, is a skip rather
// than a failure: a rewritten history legitimately holds versions that no
// longer compile.
func (s Scanner) declsAt(
	ctx context.Context,
	lang Language,
	rev, path string,
) ([]entities.Decl, error) {
	source, err := s.tree.ReadAt(ctx, rev, path)
	if err != nil {
		return nil, fmt.Errorf("read %s at %s: %w", path, rev, err)
	}

	found, err := lang.Syntax.Declarations(ctx, path, source)
	if err != nil {
		return nil, fmt.Errorf("parse %s at %s: %w", path, rev, err)
	}

	return found, nil
}

func (s Scanner) languageFor(file string) (Language, bool) {
	for _, lang := range s.langs {
		for _, ext := range lang.Extensions {
			if strings.HasSuffix(file, ext) {
				return lang, true
			}
		}
	}

	return Language{}, false
}
