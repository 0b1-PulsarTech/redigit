package planning

import (
	"fmt"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func syntheticGraph(features, declsPer int) (entities.Graph, entities.Ordering) {
	layers := []string{"api", "usecases", "repo"}
	kernel := []entities.Decl{
		decl("kernel/id.go", "NewID", "kernel"),
		decl("kernel/clock.go", "Now", "kernel"),
	}

	decls := append([]entities.Decl{}, kernel...)

	var edges []entities.Edge

	at := map[entities.DeclKey]entities.Position{}

	position := entities.Position(0)
	for _, item := range kernel {
		at[item.Key] = position
		position++
	}

	for feature := range features {
		var previous entities.Decl

		for index := range declsPer {
			layer := layers[index*len(layers)/declsPer]
			item := decl(
				fmt.Sprintf("f%d/%s/file%d.go", feature, layer, index),
				fmt.Sprintf("Fn%d", index),
				entities.UnitID(fmt.Sprintf("f%d/%s", feature, layer)),
			)

			decls = append(decls, item)
			at[item.Key] = position
			position++

			if index == 0 {
				edges = append(edges, needs(item, kernel[feature%len(kernel)]))
			} else {
				edges = append(edges, needs(previous, item))
			}

			previous = item
		}
	}

	return entities.NewGraph(decls, edges, nil), entities.Ordering{At: at}
}

func BenchmarkPlan(b *testing.B) {
	graph, order := syntheticGraph(50, 40)

	b.ReportAllocs()

	var commits int

	for b.Loop() {
		plan, err := New(Config{}).Plan(graph, order)
		if err != nil {
			b.Fatalf("Plan() = %v", err)
		}

		commits = len(plan.Commits)
	}

	b.ReportMetric(float64(commits), "commits")
	b.ReportMetric(float64(graph.Len()), "decls")
}
