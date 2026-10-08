package chunk

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// Registry lists every built-in Chunker in the order Handles should be
// tried. WholeFile is last because it claims every path, so a more specific
// chunker always gets first refusal. A caller holding an entities.ChunkMap
// prepends a MapApplier built from it; this slice alone has no knowledge of
// any particular run's map.
var Registry = []indexing.Chunker{
	SQLQueries{},
	SQLMigrations{},
	GoMod{},
	GoWork{},
	WholeFile{MaxLines: entities.DefaultMaxAddedLines},
}
