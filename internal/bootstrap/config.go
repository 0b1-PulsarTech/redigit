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

type Config struct {
	// Neither is tied to where this binary was built.
	Repo string
	Rev  string
	// Explicit, since empty is a real choice: a file behind a build constraint
	// has no edges, and Auditing.Unseen names the files that happened to.
	BuildTags []string
	// Configurable so nothing in this tool hardcodes what Go is.
	ServerCommand string
	ServerArgs    []string

	Indexing indexing.Config
	Ordering ordering.Config
	Planning planning.Config
	Auditing auditing.Config
	Rewrite  rewriting.Config

	// Writes blobs to a plain directory: duplication, but nothing is written
	// into a repository somebody else is using.
	BlobDir string

	Author AuthorConfig

	// Only refines the opaque pack's whole-file default; a path a specific
	// pack understands structurally is refused (docs/PLANO-EMISSAO.md:113-133).
	ChunkMap entities.ChunkMap

	// A module a bundler plugin makes up ("virtual:x") to the file it compiles from.
	Virtual map[string]string
}

type AuthorConfig struct {
	Name  string
	Email string
	// Fixed, not read from the clock: a rebuild whose ids change every run
	// cannot be compared with the previous round.
	Start time.Time
}

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
