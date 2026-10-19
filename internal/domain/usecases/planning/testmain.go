package planning

import (
	"cmp"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// opening is how a package's TestMain is tied to the test that opens it.
type opening struct {
	anchors []entities.Anchor
	atoms   map[entities.DeclKey]string
}

// openingTests ties TestMain to the test its package is likely to be born with,
// and makes every other test of the package wait for that one. A package whose
// setup lands hundreds of commits after its first test runs that test without
// the setup it was written against; one whose setup lands first runs it over
// nothing. The opener is the test the ordering settled earliest, because
// choosing it by name made TestMain wait for whichever suite happened to sort
// first, however late its subject was.
func openingTests(decls []entities.Decl, order entities.Ordering) opening {
	mains := map[entities.UnitID]entities.Decl{}
	tests := map[entities.UnitID][]entities.Decl{}

	for _, decl := range decls {
		if decl.Key.Kind != entities.KindFunc || !strings.HasSuffix(decl.Key.File, "_test.go") {
			continue
		}

		switch {
		case decl.Key.Name == "TestMain":
			mains[decl.Unit] = decl
		case strings.HasPrefix(decl.Key.Name, "Test"):
			tests[decl.Unit] = append(tests[decl.Unit], decl)
		}
	}

	out := opening{atoms: map[entities.DeclKey]string{}}

	for unit, main := range mains {
		members := tests[unit]
		if len(members) == 0 {
			continue
		}

		opener := members[0]

		for _, candidate := range members[1:] {
			if earlier(order, candidate, opener) {
				opener = candidate
			}
		}

		label := "test-main:" + string(unit)
		out.atoms[main.Key] = label
		out.atoms[opener.Key] = label

		// The setup has no edge to anything the opener touches, so without this
		// it is never released with the opener and trails the history alone.
		out.anchors = append(out.anchors, entities.Anchor{
			Decl: main.Key, To: opener.Key, Reason: entities.ReasonTestOf,
		})

		for _, other := range members {
			if other.Key != opener.Key {
				out.anchors = append(out.anchors, entities.Anchor{
					Decl: other.Key, To: opener.Key, Reason: entities.ReasonTestOf,
				})
			}
		}
	}

	return out
}

func earlier(order entities.Ordering, left, right entities.Decl) bool {
	if byPlace := cmp.Compare(order.Of(left.Key), order.Of(right.Key)); byPlace != 0 {
		return byPlace < 0
	}

	return left.Key.String() < right.Key.String()
}
