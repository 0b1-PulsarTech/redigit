package e2e

import (
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

func matchesAny(call facts.Call, patterns []string) bool {
	// Every pattern was checked when the rule was built, so a match error is not possible here.
	return slices.ContainsFunc(patterns, func(pattern string) bool {
		found, err := call.Matches(pattern)

		return err == nil && found
	})
}

func (rule *Rule) canRequest(call facts.Call) bool {
	if len(rule.requests) > 0 {
		return matchesAny(call, rule.requests)
	}

	pkg := strings.TrimSuffix(call.Callee, "."+call.Short())
	quiet := []string{"testing", "os", "path", "path/filepath", "log", "log/slog", "fmt", "strings"}

	return !slices.Contains(quiet, pkg) &&
		!matchesAny(call, rule.handle) && !matchesAny(call, rule.mount)
}
