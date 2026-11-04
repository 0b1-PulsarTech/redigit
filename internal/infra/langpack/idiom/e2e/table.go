package e2e

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/router"
)

type (
	served struct {
		method    string
		path      string
		providers []entities.DeclKey
	}

	table []served
)

func newTable(view router.View, top topology) table {
	var out table

	for _, route := range view.Routes {
		if route.Path == "" {
			continue
		}

		for _, via := range top.chains(route.Site) {
			out = append(out, served{
				method:    route.Method,
				path:      strings.TrimSuffix(via.prefix, "/") + route.Path,
				providers: append([]entities.DeclKey{route.Site}, via.sites...),
			})
		}
	}

	return out
}

func (rows table) waitsFor(method, url string) []entities.DeclKey {
	var literal, param []entities.DeclKey

	for _, row := range rows {
		if !answers(row.method, method) || !pathMatches(row.path, url) {
			continue
		}

		if isLiteral(row.path) {
			literal = append(literal, row.providers...)
		} else {
			param = append(param, row.providers...)
		}
	}

	if len(literal) > 0 {
		return literal
	}

	return param
}
