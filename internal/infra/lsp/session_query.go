package lsp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// References implements indexing.Resolver.
func (s *Session) References(
	ctx context.Context,
	at entities.Location,
) ([]entities.Location, error) {
	position, err := s.textDocumentPositionParams(at)
	if err != nil {
		return nil, fmt.Errorf("references: %w", err)
	}

	params := ReferenceParams{
		TextDocumentPositionParams: position,
		// The declaration itself is already known from documentSymbol;
		// references answers who calls it, not where it is.
		Context: ReferenceContext{IncludeDeclaration: false},
	}

	var result []Location
	if err := s.client.call(ctx, "textDocument/references", params, &result); err != nil {
		return nil, fmt.Errorf("references: %w", err)
	}

	return s.toEntityLocations(result)
}

// Implementations implements indexing.Resolver.
func (s *Session) Implementations(
	ctx context.Context,
	at entities.Location,
) ([]entities.Location, error) {
	params, err := s.textDocumentPositionParams(at)
	if err != nil {
		return nil, fmt.Errorf("implementations: %w", err)
	}

	var result locations
	if err := s.client.call(ctx, "textDocument/implementation", params, &result); err != nil {
		return nil, fmt.Errorf("implementations: %w", err)
	}

	return s.toEntityLocations(result)
}

// Definition implements indexing.Resolver.
func (s *Session) Definition(ctx context.Context, at entities.Location) (entities.Location, error) {
	params, err := s.textDocumentPositionParams(at)
	if err != nil {
		return entities.Location{}, fmt.Errorf("definition: %w", err)
	}

	var result locations
	if err = s.client.call(ctx, "textDocument/definition", params, &result); err != nil {
		return entities.Location{}, fmt.Errorf("definition: %w", err)
	}

	if len(result) == 0 {
		return entities.Location{}, fmt.Errorf("definition: %w", ErrNoDefinition)
	}

	found, err := s.toEntityLocation(result[0])
	if err != nil {
		return entities.Location{}, fmt.Errorf("definition: %w", err)
	}

	return found, nil
}

// DocumentSymbols answers textDocument/documentSymbol for path. It is not
// part of indexing.Resolver: declaration boundaries are Syntax's concern,
// but exposing the raw query lets a Syntax adapter fall back to it for a
// language with no dedicated parser (see docs/PLANO.md 2.1).
func (s *Session) DocumentSymbols(ctx context.Context, path string) ([]DocumentSymbol, error) {
	uri, err := pathToURI(path)
	if err != nil {
		return nil, fmt.Errorf("document symbols: %w", err)
	}

	var result []DocumentSymbol

	params := DocumentSymbolParams{TextDocument: TextDocumentIdentifier{URI: uri}}
	if err := s.client.call(ctx, "textDocument/documentSymbol", params, &result); err != nil {
		return nil, fmt.Errorf("document symbols: %w", err)
	}

	return result, nil
}

func (s *Session) textDocumentPositionParams(
	at entities.Location,
) (TextDocumentPositionParams, error) {
	uri, err := pathToURI(s.absolute(at.File))
	if err != nil {
		return TextDocumentPositionParams{}, err
	}

	return TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: at.Line, Character: at.Char},
	}, nil
}

func (s *Session) toEntityLocation(l Location) (entities.Location, error) {
	path, err := uriToPath(l.URI)
	if err != nil {
		return entities.Location{}, err
	}

	return entities.Location{
		File: s.relative(path),
		Line: l.Range.Start.Line,
		Char: l.Range.Start.Character,
	}, nil
}

func (s *Session) toEntityLocations(ls []Location) ([]entities.Location, error) {
	out := make([]entities.Location, 0, len(ls))

	for _, l := range ls {
		converted, err := s.toEntityLocation(l)
		if err != nil {
			return nil, err
		}

		out = append(out, converted)
	}

	return out, nil
}

// absolute resolves a repo-relative path against the workspace root. An
// already-absolute path is returned unchanged, so a caller working outside a
// workspace is not forced to invent one.
func (s *Session) absolute(path string) string {
	if s.root == "" || filepath.IsAbs(path) {
		return path
	}

	return filepath.Join(s.root, path)
}

// relative is the inverse, so a location leaves this package named the way the
// rest of the tool names files.
func (s *Session) relative(path string) string {
	if s.root == "" {
		return path
	}

	rel, err := filepath.Rel(s.root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}

	return rel
}
