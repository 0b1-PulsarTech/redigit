package lint

import (
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

func bornRows(graph entities.Graph, decoded []manifest.Row) (map[entities.DeclKey]int, error) {
	byText := map[string][]entities.Decl{}

	for _, decl := range graph.Decls() {
		text := decl.Key.String()
		byText[text] = append(byText[text], decl)
	}

	born := map[entities.DeclKey]int{}

	for _, row := range decoded {
		for _, chunk := range row.Chunks {
			decl, isFound := chunkDecl(byText[chunk.Key], chunk, born)
			if !isFound {
				why := "which the graph does not hold"
				if len(byText[chunk.Key]) > 0 {
					why = "which the manifest already placed in an earlier chunk: it is repeated"
				}

				return nil, fmt.Errorf(
					"%w: %w: row %d holds chunk %s, %s",
					write.ErrUsage, linting.ErrMismatch, row.Position, chunk.Key, why,
				)
			}

			born[decl.Key] = row.Position
		}
	}

	return born, nil
}

func chunkDecl(
	candidates []entities.Decl,
	chunk manifest.Chunk,
	born map[entities.DeclKey]int,
) (entities.Decl, bool) {
	// A key's text is not injective, so a span and unit break the tie when there is one.
	var fallback entities.Decl

	isFallback := false

	for _, decl := range candidates {
		if _, isBorn := born[decl.Key]; isBorn {
			continue
		}

		if decl.Span.Start == chunk.Span[0] && decl.Span.End == chunk.Span[1] &&
			string(decl.Unit) == chunk.Unit {
			return decl, true
		}

		if !isFallback {
			fallback, isFallback = decl, true
		}
	}

	return fallback, isFallback
}
