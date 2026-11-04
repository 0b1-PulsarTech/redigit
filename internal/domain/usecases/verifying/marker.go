package verifying

import "github.com/wrapped-owls/redigit/internal/domain/entities"

func isWip(subject string) bool { return entities.IsWIPSubject(subject) }
