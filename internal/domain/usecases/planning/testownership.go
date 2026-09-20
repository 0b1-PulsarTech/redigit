package planning

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func inheritTestOwnership(graph entities.Graph, own ownership) {
	for _, anchor := range graph.Anchors() {
		if anchor.Reason != entities.ReasonTestOf {
			continue
		}

		owner, found := own.owner[anchor.To]
		if !found {
			continue
		}

		// A shared target is always safe: SharedFeature never enters
		// featureNeeds, so inheriting it can never cycle two features. A real
		// feature is only safe once its own needs agree (see needsOnly).
		if owner != SharedFeature && !needsOnly(graph, anchor.Decl, own, owner) {
			continue
		}

		own.owner[anchor.Decl] = owner
		own.reach[anchor.Decl] = own.reach[anchor.To]
	}
}

func inheritTestFileOwnership(graph entities.Graph, own ownership) {
	bySourceFile := map[string][]entities.DeclKey{}
	anchored := map[entities.DeclKey]bool{}

	for _, decl := range graph.Decls() {
		if !decl.IsTest {
			bySourceFile[decl.Key.File] = append(bySourceFile[decl.Key.File], decl.Key)
		}
	}

	for _, anchor := range graph.Anchors() {
		if anchor.Reason == entities.ReasonTestOf {
			anchored[anchor.Decl] = true
		}
	}

	for _, decl := range graph.Decls() {
		// An anchored test is settled, shared or not: re-guessing from the
		// twin file is what let a shared test get claimed twice.
		if !decl.IsTest || anchored[decl.Key] || own.owner[decl.Key] != SharedFeature {
			continue
		}

		twin, ok := strings.CutSuffix(decl.Key.File, "_test.go")
		if !ok {
			continue
		}

		siblings := bySourceFile[twin+".go"]

		owners := map[FeatureID]bool{}

		for _, sibling := range siblings {
			if owner := own.owner[sibling]; owner != SharedFeature {
				owners[owner] = true
			}
		}

		owner := soleOwner(owners)

		// A need reaching outside the twin file's feature is a real
		// cross-feature edge; folding the test in anyway would put two
		// features that never needed each other in a cycle.
		if owner == SharedFeature || !needsOnly(graph, decl.Key, own, owner) {
			continue
		}

		own.owner[decl.Key] = owner
		own.reach[decl.Key] = len(owners)
	}
}

func needsOnly(graph entities.Graph, key entities.DeclKey, own ownership, owner FeatureID) bool {
	for _, edge := range graph.Needs(key) {
		if provider := own.owner[edge.Provider]; provider != SharedFeature && provider != owner {
			return false
		}
	}

	return true
}
