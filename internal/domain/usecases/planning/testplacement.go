package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// LateFeature holds the tests that exercise more than one feature. Nothing
// needs a test, so they are born after every feature, in the trailing layer.
const LateFeature FeatureID = "~late"

// placeStrandedTests keeps a shared test from dragging its subjects into the
// shared layer.
//
// A test reached by no entry point, or anchored to a shared declaration, is
// shared, and sharing it made everything it calls shared too: one integration
// test of a use case moved that use case, its helpers and every file they
// touch into the layer born before all features (588 files in one commit on
// the real tree, 181 of them only through a test). The edge runs from the test
// to what it exercises, so the test moves to its subject: the feature that owns
// everything non-shared it uses, or the trailing layer when those are several.
// A test using only shared declarations stays shared, where it is born as early
// as its subjects.
func placeStrandedTests(graph entities.Graph, own ownership) {
	for changed := true; changed; {
		changed = false

		for _, decl := range graph.Decls() {
			if !decl.IsTest || own.owner[decl.Key] != SharedFeature {
				continue
			}

			target, stranded := subjectOwner(graph, own, decl.Key)
			if !stranded {
				continue
			}

			own.owner[decl.Key] = target
			changed = true
		}
	}
}

// subjectOwner is the one feature that owns every non-shared declaration key
// needs, LateFeature when there are several, and false when it needs none.
func subjectOwner(graph entities.Graph, own ownership, key entities.DeclKey) (FeatureID, bool) {
	var (
		found  bool
		target FeatureID
	)

	for _, edge := range graph.Needs(key) {
		owner, known := own.owner[edge.Provider]
		if !known || owner == SharedFeature {
			continue
		}

		if owner == LateFeature || (found && owner != target) {
			return LateFeature, true
		}

		found = true
		target = owner
	}

	return target, found
}
