package rewriting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/subjecting"
)

var ErrSubjects = errors.New("invalid subjects")

func Messages(rows []Row, subjects map[string]string, allowDraft bool) ([]string, error) {
	slots := make([]subjecting.Slot, 0, len(rows))
	for _, row := range rows {
		slots = append(slots, subjecting.Slot{Position: row.Position, ID: row.ID})
	}

	hands, problems := subjecting.Resolve(slots, subjects)
	if len(problems) > 0 {
		return nil, fmt.Errorf(
			"%w: %s key %q", ErrSubjects, problems[0].Kind, problems[0].Key,
		)
	}

	messages := make([]string, 0, len(rows))

	for idx, row := range rows {
		subject := row.Subject
		if hands[idx].Found {
			subject = hands[idx].Subject
		}

		if !allowDraft && entities.IsDraft(subject) {
			return nil, fmt.Errorf(
				"%w: position %d still carries a generated subject",
				apperr.ErrIncomplete, row.Position,
			)
		}

		if row.IsWIP {
			subject = subjecting.Mark(subject)
		}

		messages = append(messages, strings.TrimRight(subject, "\n")+"\n")
	}

	return messages, nil
}
