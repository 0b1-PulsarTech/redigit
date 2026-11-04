package lineage

import "github.com/wrapped-owls/redigit/internal/domain/entities"

type Renames map[entities.DeclKey][]Rename

func (result Result) Renames() Renames {
	renames := Renames{}

	for _, birth := range result.Births {
		for _, rename := range birth.Via {
			renames[rename.From] = append(renames[rename.From], rename)
		}
	}

	return renames
}

func (renames Renames) Final(key entities.DeclKey, after entities.Position) entities.DeclKey {
	for {
		var next Rename

		var isFound bool

		for _, rename := range renames[key] {
			// The earliest rename past the position is the next link of the chain.
			if rename.At > after && (!isFound || rename.At < next.At) {
				next, isFound = rename, true
			}
		}

		if !isFound {
			return key
		}

		key, after = next.To, next.At
	}
}
