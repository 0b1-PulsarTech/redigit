package linting

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type syntaxMock struct {
	statements map[string]Statement
	exempt     map[string]string
	err        error
}

var _ Syntax = syntaxMock{}

func (mock syntaxMock) Statements(
	_ context.Context,
	_ string,
	spans map[entities.DeclKey]entities.Span,
) (map[entities.DeclKey]Statement, error) {
	if mock.statements == nil {
		return nil, mock.err
	}

	out := map[entities.DeclKey]Statement{}

	for key := range spans {
		if statement, found := mock.statements[key.Name]; found {
			out[key] = statement
		}
	}

	return out, mock.err
}

func (mock syntaxMock) Exempt(decl entities.Decl) string {
	return mock.exempt[decl.Key.Name]
}
