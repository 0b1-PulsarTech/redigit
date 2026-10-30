package verifying

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func isWip(subject string) bool {
	body := strings.TrimPrefix(subject, entities.DraftMarker)

	if _, description, hasType := strings.Cut(body, ": "); hasType {
		return strings.HasPrefix(description, entities.WIPMarker)
	}

	return strings.HasPrefix(body, entities.WIPMarker)
}
