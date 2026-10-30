package lineage

import "github.com/wrapped-owls/redigit/internal/domain/entities"

type (
	Birth struct {
		At  entities.Position
		Via []Rename
	}

	Rename struct {
		At        entities.Position
		From, To  entities.DeclKey
		FileScore int
		TextScore int
	}

	Result struct {
		Births     map[entities.DeclKey]Birth
		Unreadable []string
	}
)

func (rename Rename) ByText() bool { return rename.TextScore > 0 }
