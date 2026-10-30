package lineage

import (
	"context"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type (
	pairing struct {
		hist  *history
		units []parsed
	}

	declPair struct {
		from, to  entities.DeclKey
		textScore int
	}

	nameKind struct {
		kind entities.DeclKind
		name string
	}

	candidate struct {
		declPair

		oldIdx, newIdx int
	}
)

func (scanner Scanner) inherit(
	ctx context.Context,
	births map[entities.DeclKey]Birth,
	seen sighting,
	pair pairing,
) error {
	oldDecls := pair.units[seen.move.fromUnit].cuts
	newDecls := pair.units[seen.unit].cuts

	pairs, oldLeft, newLeft := matchNames(oldDecls, newDecls)
	if len(oldLeft) > 0 && len(newLeft) > 0 {
		blobs := [2]string{
			pair.hist.units[seen.move.fromUnit].blob, pair.hist.units[seen.unit].blob,
		}

		byText, err := scanner.matchText(ctx, blobs, oldLeft, newLeft)
		if err != nil {
			return err
		}

		pairs = append(pairs, byText...)
	}

	for _, found := range pairs {
		prior, ok := births[found.from]
		if !ok {
			continue
		}

		if own, owned := births[found.to]; owned && own.At <= prior.At {
			continue
		}

		births[found.to] = Birth{At: prior.At, Via: append(slices.Clone(prior.Via), Rename{
			At: seen.at, From: found.from, To: found.to,
			FileScore: seen.move.fileScore, TextScore: found.textScore,
		})}
	}

	return nil
}

func matchNames(
	oldDecls, newDecls []entities.Decl,
) (pairs []declPair, oldLeft, newLeft []entities.Decl) {
	waiting := map[nameKind][]int{}

	for idx, decl := range oldDecls {
		name := nameKind{kind: decl.Key.Kind, name: decl.Key.Name}
		waiting[name] = append(waiting[name], idx)
	}

	taken := make([]bool, len(oldDecls))

	for _, decl := range newDecls {
		name := nameKind{kind: decl.Key.Kind, name: decl.Key.Name}
		if queue := waiting[name]; len(queue) > 0 {
			taken[queue[0]] = true
			waiting[name] = queue[1:]
			pairs = append(pairs, declPair{
				from: oldDecls[queue[0]].Key, to: decl.Key,
			})

			continue
		}

		newLeft = append(newLeft, decl)
	}

	for idx, decl := range oldDecls {
		if !taken[idx] {
			oldLeft = append(oldLeft, decl)
		}
	}

	return pairs, oldLeft, newLeft
}
