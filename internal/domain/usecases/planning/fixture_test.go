package planning

import (
	"fmt"
	"strconv"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func proseEdge(dependent, provider entities.Decl) entities.Edge {
	return entities.Edge{
		Dependent: dependent.Key,
		Provider:  provider.Key,
		Reason:    entities.ReasonProse,
	}
}

func keysOf(decls []entities.Decl) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(decls))
	for _, item := range decls {
		out = append(out, item.Key)
	}

	return out
}

// rooted is twoFeatures plus a composition root that wires both handlers and
// enough registrations of its own to read as the program's assembly.
func rooted() (entities.Graph, entities.Ordering, map[string]entities.Decl) {
	base, order, named := twoFeatures()
	root := decl("cmd/app/main.go", "Run", "cmd/app")

	decls := base.Decls()
	edges := base.Edges()
	at := map[entities.DeclKey]entities.Position{}

	for _, d := range decls {
		at[d.Key] = order.Of(d.Key)
	}

	decls = append(decls, root)
	edges = append(
		edges,
		needs(root, named["leadAPI"]),
		needs(root, named["billAPI"]),
	)
	at[root.Key] = entities.Position(len(at))

	for index := range 20 {
		ballast := decl("cmd/app/bind.go", fmt.Sprintf("Bind%d", index), "cmd/app")
		decls = append(decls, ballast)
		edges = append(edges, needs(root, ballast))
		at[ballast.Key] = entities.Position(len(at))
	}

	named["root"] = root

	return entities.NewGraph(decls, edges, nil), entities.Ordering{At: at}, named
}

func statement(function, file string, index int) entities.Decl {
	return entities.Decl{
		Key: entities.DeclKey{
			File: file,
			Kind: entities.KindStmt,
			Name: function + "#" + strconv.Itoa(index),
		},
		Unit: "cmd/app",
	}
}

func prose(name string) entities.Decl {
	found := decl("CHANGELOG.md", name, "")
	found.Key.Kind = entities.KindOther

	return found
}

func tiedFeatures() (entities.Graph, entities.Ordering) {
	helper := decl("kernel/helper.go", "Helper", "kernel")
	decls := []entities.Decl{helper}
	at := map[entities.DeclKey]entities.Position{helper.Key: 0}

	edges := make([]entities.Edge, 0, 16)

	features := []struct {
		name string
		spot entities.Position
	}{{"lead", 3}, {"bill", 5}}

	for _, feature := range features {
		entry := decl(
			feature.name+"/api/handler.go",
			"Handle",
			entities.UnitID(feature.name+"/api"),
		)
		block := make([]entities.Decl, 0, 6)

		for index := range 6 {
			member := decl(
				fmt.Sprintf("%s/usecases/part%d.go", feature.name, index%3),
				fmt.Sprintf("Step%d", index),
				entities.UnitID(feature.name+"/usecases"),
			)
			member.IsInterface = index == 5
			block = append(block, member)
			at[member.Key] = feature.spot
		}

		decls = append(append(decls, entry), block...)
		at[entry.Key] = feature.spot + 1
		entryEdges := []entities.Edge{
			needs(entry, block[0]),
			needs(block[0], helper),
			needs(block[4], helper),
		}
		edges = append(edges, entryEdges...)

		for index := range 4 {
			edges = append(edges, needs(block[index], block[index+1]))
		}

		edges = append(edges, entities.Edge{
			Dependent: block[2].Key, Provider: block[5].Key, Reason: entities.ReasonImplements,
		})
	}

	return entities.NewGraph(decls, edges, nil), entities.Ordering{At: at}
}
