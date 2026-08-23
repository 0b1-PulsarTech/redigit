package bootstrap

import (
	"time"

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
	// inferred because leaving it empty is a real choice with a silent cost:
	// a file behind a build constraint is invisible to reference queries and
	// nothing reports that it was skipped.
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
		Author: AuthorConfig{
			Name:  "redigit",
			Email: "redigit@localhost",
			Start: time.Unix(0, 0).UTC(),
		},
	}
}
