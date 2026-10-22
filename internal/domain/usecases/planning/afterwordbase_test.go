package planning

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestPlannerBearsTheTitleWithTheFirstEntry(t *testing.T) {
	t.Parallel()

	graph, order, parts := twoFeatures()
	title := prose("<file>")
	lead := prose("<file>#2")
	bill := prose("<file>#3")

	decls := append(graph.Decls(), title, lead, bill)
	edges := append(
		graph.Edges(),
		edgeOf(lead, parts["leadAPI"], entities.ReasonProse),
		edgeOf(lead, title, entities.ReasonProse),
		edgeOf(bill, parts["billAPI"], entities.ReasonProse),
		edgeOf(bill, title, entities.ReasonProse),
	)

	order.At[title.Key], order.At[lead.Key], order.At[bill.Key] = 9, 7, 8

	plan, err := New(Config{}).Plan(entities.NewGraph(decls, edges, nil), order)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	bornAt := map[entities.DeclKey]int{}

	for index, commit := range plan.Commits {
		for _, key := range commit.Decls {
			bornAt[key] = index
		}
	}

	first := min(bornAt[lead.Key], bornAt[bill.Key])
	if bornAt[title.Key] != first {
		t.Fatalf("the title is at %d, the first entry at %d", bornAt[title.Key], first)
	}

	for _, entry := range []struct{ entry, route entities.Decl }{
		{lead, parts["leadAPI"]}, {bill, parts["billAPI"]},
	} {
		if bornAt[entry.entry.Key] < bornAt[entry.route.Key] {
			t.Fatalf("%s is born before the route it records", entry.entry.Key)
		}
	}
}

func prose(name string) entities.Decl {
	found := decl("CHANGELOG.md", name, "")
	found.Key.Kind = entities.KindOther

	return found
}
