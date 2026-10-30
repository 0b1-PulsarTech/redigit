package rewriting

import (
	"fmt"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func Messages(rows []Row, subjects map[string]string, allowDraft bool) ([]string, error) {
	messages := make([]string, 0, len(rows))

	for _, row := range rows {
		subject, found := subjects[fmt.Sprint(row.Position)]
		if !found {
			subject = row.Subject
		}

		if !allowDraft && entities.IsDraft(subject) {
			return nil, fmt.Errorf(
				"%w: position %d still carries a generated subject",
				apperr.ErrIncomplete, row.Position,
			)
		}

		messages = append(messages, strings.TrimRight(subject, "\n")+"\n")
	}

	return messages, nil
}
