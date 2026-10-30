package gofacts

import (
	"errors"
	"fmt"
	"path"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

var ErrNotFunc = errors.New("key names no function in the loaded packages")

type Index struct {
	calls map[entities.DeclKey][]facts.Call
	ctors map[entities.DeclKey][]facts.TypeRef
	impl  *implementors
}

func (index *Index) Calls(site entities.DeclKey) []facts.Call {
	return index.calls[site] // Shared with the index: callers must treat it as read-only.
}

func (index *Index) CtorArgTypes(fn entities.DeclKey) ([]facts.TypeRef, error) {
	taken, known := index.ctors[fn]
	if !known {
		return nil, fmt.Errorf("constructor arguments of %s: %w", fn, ErrNotFunc)
	}

	return taken, nil
}

func (index *Index) Implementors(iface entities.DeclKey) []entities.DeclKey {
	return index.impl.of(iface)
}

func (index *Index) StmtCallsInto(stmt entities.DeclKey, pattern string) ([]facts.Call, error) {
	// An empty name still reports a malformed pattern, so the answer does not depend on the calls.
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, fmt.Errorf("calls of %s into %q: %w", stmt, pattern, err)
	}

	var into []facts.Call

	for _, call := range index.calls[stmt] {
		matched, err := call.Matches(pattern)
		if err != nil {
			return nil, fmt.Errorf("calls of %s: %w", stmt, err)
		}

		if matched {
			into = append(into, call)
		}
	}

	return into, nil
}
