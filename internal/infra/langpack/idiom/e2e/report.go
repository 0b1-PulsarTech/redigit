package e2e

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
)

func (rule *Rule) report(count tally, routes, edges int) {
	urls := make([]string, 0, len(count.unknown))
	for url := range count.unknown {
		urls = append(urls, url)
	}

	slices.Sort(urls)

	summary := fmt.Sprintf(
		"%s: %d test declarations in dirs, %d without call facts (nested module?), "+
			"%d routes, %d edges, %d urls with no known route path",
		rule.name, count.listening, count.blind, routes, edges, len(urls),
	)

	if edges == 0 && count.listening > 0 {
		rule.logger.Warn(summary)
	} else {
		rule.logger.Info(summary)
	}

	if len(urls) > 0 {
		rule.logger.Warn(
			rule.name+": no known route path for these urls, so no edge was added; "+
				"a group variable, a prefix that is not a constant or a URL built elsewhere is not seen",
			slog.Int("urls", len(urls)),
			slog.String("list", strings.Join(urls, " | ")),
		)
	}
}
