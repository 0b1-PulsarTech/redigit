package bootstrap

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
	"github.com/wrapped-owls/redigit/internal/infra/inputs"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

var ErrConfig = errors.New("invalid config")

// ErrChunkMapUnknownPath reports a chunk map entry naming a path outside the
// tree being read. A wrong map is worse than no map, and a path that is not
// there at all is the plainest way a map can be wrong.
var ErrChunkMapUnknownPath = errors.New("chunk map names a path outside the tree")

// ErrChunkMapConflict reports a chunk map entry naming a path a language pack
// already claims structurally. The map only refines the opaque pack's
// whole-file default (docs/PLANO-EMISSAO.md:113-133); it does not override a
// pack that already reads the format.
var ErrChunkMapConflict = errors.New("chunk map path is already claimed by a language pack")

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
	Resolver      string // ResolverLSP, or ResolverTyped; "" (the zero value) means typed

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

	// Built at load time, so a bad entry fails before any stage starts.
	Rules       []rules.Rule
	ConfigPath  string
	ConfigBytes []byte // what ConfigPath was parsed from; the plan cache keys these
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

type setting struct {
	language string
	key      string
	flag     string
	apply    func(raw json.RawMessage, cfg *Config) error
}

func settings() []setting {
	// One table decodes, applies and validates, so no second key list can drift.
	return []setting{
		{goLanguage, "tags", "tags", func(raw json.RawMessage, cfg *Config) error {
			var tags []*string
			if err := rules.DecodeStrict(raw, &tags, "an array of strings"); err != nil {
				return err //nolint:wrapcheck // rules.Expected words the setting error
			}

			joined := make([]string, len(tags))

			for idx, tag := range tags {
				if tag == nil {
					return rules.Expected("an array of strings")
				}

				joined[idx] = *tag
			}

			var err error

			cfg.BuildTags, err = inputs.ParseTags(strings.Join(joined, ","))

			return err //nolint:wrapcheck // settingError names the setting once
		}},
		{webLanguage, "virtual", "virtual", func(raw json.RawMessage, cfg *Config) error {
			var virtual map[string]*string
			if err := rules.DecodeStrict(raw, &virtual, "an object of strings"); err != nil {
				return err //nolint:wrapcheck // rules.Expected words the setting error
			}

			cfg.Virtual = make(map[string]string, len(virtual))

			for _, name := range slices.Sorted(maps.Keys(virtual)) {
				if virtual[name] == nil {
					return rules.Expected("an object of strings")
				}

				if err := inputs.AddVirtual(cfg.Virtual, name, *virtual[name]); err != nil {
					return err //nolint:wrapcheck // settingError names the setting once
				}
			}

			return nil
		}},
	}
}

func SettingFlags() []string {
	flags := make([]string, 0, len(settings()))

	for _, entry := range settings() {
		flags = append(flags, entry.flag)
	}

	return flags
}
