package anchorprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/infra/checkrun"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/slots"
	"github.com/wrapped-owls/redigit/internal/infra/verifycache"
)

type Settings struct {
	Repo     string
	Rev      string
	CacheDir string
	Tags     []string
	Verify   json.RawMessage
	Renderer Renderer
}

func Open(ctx context.Context, settings Settings) (*Probe, error) {
	const slotsInDir = "anchor-slots"

	gitDir, err := slots.GitDir(ctx, settings.Repo)
	if err != nil {
		return nil, fmt.Errorf("find git dir: %w", err)
	}

	root := settings.CacheDir
	if root == "" {
		if root, err = verifycache.DefaultDir(); err != nil {
			return nil, fmt.Errorf("find cache dir: %w", err)
		}
	}

	verify, err := configure(settings.Tags, settings.Verify)
	if err != nil {
		return nil, err
	}

	builder, err := checkrun.New(checkrun.Config{Languages: verify.languages, CacheDir: root})
	if err != nil {
		return nil, fmt.Errorf("create builder: %w", err)
	}

	trees := slots.NewTrees(gitDir, filepath.Join(root, slotsInDir))
	probe := New(ctx, Config{
		Commits:  slots.NewCommits(gitDir),
		Checkout: trees,
		Modules:  slots.NewModules(gitDir, verify.markers),
		Builder:  builder,
		Renderer: settings.Renderer,
		Rev:      settings.Rev,
		Exclude:  verify.exclude,
	})
	probe.release = trees.Close

	return probe, nil
}

func (probe *Probe) Close(ctx context.Context) error {
	if probe.release == nil {
		return nil
	}

	return probe.release(ctx)
}
