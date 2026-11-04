package router

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

type Route struct {
	Method  string
	Path    string
	Handler entities.DeclKey
	Site    entities.DeclKey
}

func matchesAny(call facts.Call, patterns []string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := call.Matches(pattern)
		if err != nil {
			return false, fmt.Errorf("match call: %w", err)
		}

		if matched {
			return true, nil
		}
	}

	return false, nil
}

func routeOf(call facts.Call) Route {
	short := call.Short()
	verb := strings.ToUpper(short[strings.LastIndex(short, ".")+1:])
	route := Route{Method: verb, Site: call.Site}

	// Only the last func argument is the handler; earlier ones are read as middleware.
	if len(call.FuncArgs) > 0 {
		route.Handler = call.FuncArgs[len(call.FuncArgs)-1]
	}

	for _, literal := range call.Literals {
		if method, pattern, found := strings.Cut(literal, " /"); found && isVerb(method) {
			route.Method, route.Path = method, "/"+pattern

			return route
		}

		if strings.HasPrefix(literal, "/") {
			route.Path = literal

			return route
		}
	}

	return route
}

func isVerb(word string) bool {
	return word != "" && strings.ToUpper(word) == word && !strings.ContainsAny(word, " /")
}

func prefixOf(cfg Config, call facts.Call) string {
	if call.Within == nil {
		return ""
	}

	if mounted, err := matchesAny(*call.Within, cfg.Mount); err != nil || !mounted {
		return ""
	}

	at := slices.IndexFunc(call.Within.Literals, func(literal string) bool {
		return strings.HasPrefix(literal, "/")
	})
	if at < 0 {
		return ""
	}

	return call.Within.Literals[at]
}

func joinPath(prefix, route string) string {
	if route == "" {
		return ""
	}

	return strings.TrimSuffix(prefix, "/") + route
}
