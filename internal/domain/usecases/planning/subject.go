package planning

import (
	"fmt"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

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
		entities.DraftMarker, kind, scope, describe(keys),
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
