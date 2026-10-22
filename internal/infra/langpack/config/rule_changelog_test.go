package config

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// The title and the first entry shared one piece, so the title was born with
// whichever entry happened to be listed first, long after the others.
func TestChangelogRuleMakesEveryRouteEntryWaitForTheTitle(t *testing.T) {
	t.Parallel()

	changelog := "# Changelog\n\n## one\n\n`GET /leads`\n\n## two\n\nnothing\n"
	group := "func (h Handler) GroupName() string { return \"/leads\" }\n"
	snap := treeWith(t, map[string]string{"CHANGELOG.md": changelog, "leads/router.go": group})

	title := piece("CHANGELOG.md", "<file>", entities.Span{Start: 0, End: 13})
	one := piece("CHANGELOG.md", "<file>#2", entities.Span{Start: 13, End: 40})
	two := piece("CHANGELOG.md", "<file>#3", entities.Span{Start: 40, End: len(changelog)})
	route := entities.Decl{
		Key: entities.DeclKey{
			File: "leads/router.go",
			Kind: entities.KindMethod,
			Name: "Handler.GroupName",
		},
		Span: entities.Span{Start: 0, End: len(group)},
	}

	edges, err := (changelogRule{}).Edges(
		t.Context(),
		snap,
		[]entities.Decl{title, one, two, route},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, one.Key, route.Key) || !edgeBetween(edges, one.Key, title.Key) {
		t.Error("the entry does not wait for both its route and the title")
	}

	if edgeBetween(edges, two.Key, title.Key) || edgeBetween(edges, title.Key, route.Key) {
		t.Error("an entry naming no route, or the title, waits for something")
	}
}
