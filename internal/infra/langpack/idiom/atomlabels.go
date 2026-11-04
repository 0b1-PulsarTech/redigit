package idiom

import (
	"strconv"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func mergedAtoms(groups []map[entities.DeclKey]string) map[entities.DeclKey]string {
	atoms := map[entities.DeclKey]string{}

	for index, group := range groups {
		for key, label := range group {
			if index > 0 {
				label += "#rules[" + strconv.Itoa(index) + "]"
			}

			atoms[key] = label
		}
	}

	return atoms
}
