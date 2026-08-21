package planning

import (
	"fmt"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// DraftMarker prefixes every subject the planner invents. A generated subject
// is evidence for a reviewer, not a commit message: the previous rebuild ended
// up with 561 placeholder subjects that had to be authored afterwards, and the
// ones that slipped through described work that had moved elsewhere. The marker
// is what lets the writer refuse to commit an unreviewed one.
const DraftMarker = "DRAFT: "

// IsDraft reports whether a subject still needs a human or an agent to write it.
func IsDraft(subject string) bool { return strings.HasPrefix(subject, DraftMarker) }

func draftSubject(shape entities.CommitShape, feature FeatureID, keys []entities.DeclKey) string {
	scope := string(feature)
	if idx := strings.LastIndex(scope, "/"); idx >= 0 {
		scope = scope[idx+1:]
	}

	if scope == "" {
		scope = "shared"
	}

	kind := "feat"
	if shape == entities.ShapeLayer {
		kind = "chore"
	}

	return fmt.Sprintf(
		"%s%s(%s): %s",
		DraftMarker, kind, scope, describe(keys),
	)
}

// describe names what the commit actually declares, which is the evidence a
// reviewer needs to write a truthful subject.
func describe(keys []entities.DeclKey) string {
	const shown = 3

	names := make([]string, 0, shown)

	for _, key := range keys {
		if len(names) == shown {
			break
		}

		names = append(names, key.Name)
	}

	if len(keys) > len(names) {
		return fmt.Sprintf("%s and %d more", strings.Join(names, ", "), len(keys)-len(names))
	}

	return strings.Join(names, ", ")
}
