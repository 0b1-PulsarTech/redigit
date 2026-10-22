package bootstrap

import (
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

// Config is everything a run needs, read once and treated as read-only after.
type Config struct {
	// Repo is the repository to read, and Rev the revision holding the tree
	// to rebuild. Neither is tied to where this binary was built.
	Repo string
	Rev  string
	// BuildTags is passed to the language server. It is explicit rather than
	// inferred because leaving it empty is a real choice with a cost: a file
	// behind a build constraint is invisible to reference queries, so its
	// declarations arrive with no edges. Auditing.Unseen names the files that
	// happened to, which is what keeps the cost from being silent.
	BuildTags []string
	// ServerCommand and ServerArgs launch the language server. Configurable
	// so nothing in this tool hardcodes what Go is.
	ServerCommand string
	ServerArgs    []string

	Indexing indexing.Config
	Ordering ordering.Config
	Planning planning.Config
	Auditing auditing.Config
	Rewrite  rewriting.Config

	// BlobDir, when set, writes blobs to a plain directory instead of the
	// repository's object database. It costs duplication and buys the promise
	// that nothing is written into a repository somebody else is using.
	BlobDir string

	Author AuthorConfig

	// ChunkMap is the operator-supplied escape hatch for a format this tool
	// cannot parse (docs/PLANO-EMISSAO.md:113-133). It only refines the
	// opaque pack's whole-file default; a path a specific pack already
	// understands structurally is refused rather than overridden.
	ChunkMap entities.ChunkMap
}

// AuthorConfig is who a rebuilt history is attributed to, and from when. The
// timestamp is fixed rather than read from the clock so two runs of the same
// plan produce the same commits; a rebuild whose identifiers change on every
// run cannot be compared with the previous round.
type AuthorConfig struct {
	Name  string
	Email string
	Start time.Time
}

// Defaults is the configuration a run starts from before flags narrow it.
func Defaults() Config {
	return Config{
		Rev:           "HEAD",
		ServerCommand: "gopls",
		ServerArgs:    []string{"-mode=stdio"},
		Planning:      planning.Config{MinCommitLines: entities.DefaultMinCommitLines},
		Author: AuthorConfig{
			Name:  "redigit",
			Email: "redigit@localhost",
			Start: time.Unix(0, 0).UTC(),
		},
	}
}
