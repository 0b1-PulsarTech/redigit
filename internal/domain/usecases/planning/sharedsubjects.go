package planning

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// sharedSubjects is, per test, the subjects it waits for: those born outside a
// feature. A test is anchored to every declaration of the file it exercises,
// and one piece of that file may be read by a single feature far down the
// order; waiting for it would leave a suite that needs none of it hundreds of
// commits behind the code it tests.
func sharedSubjects(
	graph entities.Graph,
	own ownership,
) map[entities.DeclKey][]entities.DeclKey {
	out := map[entities.DeclKey][]entities.DeclKey{}

	for test, subjects := range TestSubjects(graph) {
		for _, subject := range subjects {
			if own.owner[subject] == SharedFeature {
				out[test] = append(out[test], subject)
			}
		}
	}

	return out
}
