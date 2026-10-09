package web

import (
	"context"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// routeRule makes a SvelteKit page or layout wait for the server module beside
// it. Nothing imports that module: the router pairs them by file name, and the
// component reads what its load returns and posts to its actions.
type routeRule struct{}

func (routeRule) Name() string { return "route" }

var routeComponents = map[string]string{"+page.svelte": "+page", "+layout.svelte": "+layout"}

var serverSuffixes = []string{".server.dart", ".server.ts", ".server.js", tsExt, jsExt}

func (routeRule) Edges(_ context.Context, run *index) ([]entities.Edge, error) {
	var out []entities.Edge

	for _, file := range run.scripts {
		stem, isRoute := routeComponents[path.Base(file)]
		if !isRoute {
			continue
		}

		dependent, _ := run.first(file)

		for _, suffix := range serverSuffixes {
			for _, provider := range run.byFile[path.Join(path.Dir(file), stem+suffix)] {
				out = append(out, entities.Edge{
					Dependent: dependent,
					Provider:  provider.Key,
					Reason:    entities.ReasonReference,
				})
			}
		}
	}

	return out, nil
}

func (routeRule) Anchors(context.Context, *index) ([]entities.Anchor, error) { return nil, nil }

const routesDir = "src/routes/"

// routeFor finds the page a test visits at appPath, among the pages of the
// package holding root. A bracketed segment matches any one segment, and the
// page with the fewest of them wins.
func (run *index) routeFor(root, appPath string) (string, bool) {
	want := strings.Split(strings.Trim(appPath, "/"), "/")
	best, bestParams := "", len(want)+1

	for _, file := range run.scripts {
		rest, isPage := strings.CutPrefix(file, path.Join(root, routesDir)+"/")
		if !isPage || path.Base(file) != "+page.svelte" {
			continue
		}

		segments := strings.Split(path.Dir(rest), "/")
		if params, fits := segmentsMatch(segments, want); fits && params < bestParams {
			best, bestParams = file, params
		}
	}

	return best, best != ""
}

func segmentsMatch(route, want []string) (int, bool) {
	if len(route) != len(want) {
		return 0, false
	}

	params := 0

	for index, segment := range route {
		switch {
		case strings.HasPrefix(segment, "["):
			params++
		case segment != want[index]:
			return 0, false
		}
	}

	return params, true
}
