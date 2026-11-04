package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
	"github.com/wrapped-owls/redigit/internal/infra/anchorprobe"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/preimage"
)

type (
	lineageSection struct {
		Follow  bool             `json:"follow"`
		Policy  anchoring.Policy `json:"policy"`
		Anchors []anchoring.Spec `json:"anchors"`
	}

	lineageScan func(context.Context, *Config, stageEnv, lineage.Config) (lineage.Result, error)

	anchorOpen func(context.Context, *Config, stageEnv) (anchoring.Config, closers, error)

	lineageDeps struct {
		scan lineageScan
		open anchorOpen
	}

	lineageOrderer struct {
		ordering.Orderer

		result lineage.Result
	}
)

func lineageHook(ctx context.Context, cfg *Config, env stageEnv, built *stages) error {
	return lineageStage(ctx, cfg, env, built, lineageDeps{scan: scanHistory, open: openProbe})
}

func lineageStage(
	ctx context.Context,
	cfg *Config,
	env stageEnv,
	built *stages,
	deps lineageDeps,
) error {
	// Decoded in every mode, so a bad section fails before any indexing starts.
	section, err := decodeLineageSection(cfg.Sections.Lineage)
	if err != nil {
		return err
	}

	anchors, err := anchoring.Locate(section.Anchors, env.opened.snapshot.History)
	if err != nil {
		return fmt.Errorf("%w: lineage anchors: %w", ErrConfig, err)
	}

	if cfg.Mode != ModeLineage {
		if len(anchors) > 0 {
			slog.Warn("anchors: ignored outside lineage mode", slog.Int("count", len(anchors)))
		}

		return nil
	}

	result, err := deps.scan(ctx, cfg, env, lineage.Config{
		Languages: lineageLanguages(env.specific), Follow: section.Follow,
	})
	if err != nil {
		return fmt.Errorf("scan lineage: %w", err)
	}

	if len(result.Unreadable) > 0 {
		slog.Warn("lineage: revisions the scan could not use",
			slog.Int("count", len(result.Unreadable)),
			slog.String("first", result.Unreadable[0]),
		)
	}

	floored := cfg.Ordering
	floored.Floor = lineage.Floor(result)
	// The full Result rides on the orderer for the anchors stage to read.
	built.orderer = lineageOrderer{Orderer: ordering.New(floored), result: result}

	if len(anchors) == 0 {
		return nil
	}

	anchored, held, err := deps.open(ctx, cfg, env)
	if err != nil {
		return fmt.Errorf("open anchors: %w", err)
	}

	anchored.Inner, anchored.Policy, anchored.Anchors = built.planner, section.Policy, anchors
	anchored.Floor, anchored.Renames, anchored.Bound = floored.Floor, result.Renames(), ctx
	anchored.Languages, anchored.Finals = lineageLanguages(env.specific), env.tree.reader

	built.closers = append(built.closers, held...)
	built.planner = anchoring.New(anchored)

	return nil
}

func openProbe(ctx context.Context, cfg *Config, env stageEnv) (anchoring.Config, closers, error) {
	renderers := renderersByExtension(env.specific)
	settings := anchorprobe.Settings{
		Repo: env.root, Rev: cfg.Rev, Tags: cfg.BuildTags, Verify: cfg.Sections.Verify,
		Renderer: newRenderer(renderers[".go"], renderers),
	}

	probe, err := anchorprobe.Open(ctx, settings)
	if err != nil {
		return anchoring.Config{}, nil, fmt.Errorf("open probe: %w", err)
	}

	reader := preimage.NewReader(env.root)
	opened := anchoring.Config{Revisions: reader, Probe: probe, Reader: probe}

	return opened, closers{probe, reader}, nil
}

func scanHistory(
	ctx context.Context,
	cfg *Config,
	env stageEnv,
	scanCfg lineage.Config,
) (lineage.Result, error) {
	revisions := batch.NewRevisions(env.root, cfg.Rev)
	result, err := lineage.NewScanner(revisions, scanCfg).Scan(ctx, env.opened.snapshot)

	// Closed on every path: the stream is not needed once the scan is over.
	return result, errors.Join(err, revisions.Close())
}

func lineageLanguages(specific []indexing.Language) []lineage.Language {
	languages := make([]lineage.Language, 0, len(specific))

	for _, language := range specific {
		if language.Syntax != nil {
			languages = append(languages, lineage.Language{
				Extensions: language.Extensions, Parser: language.Syntax,
			})
		}
	}

	return languages
}

func decodeLineageSection(raw json.RawMessage) (lineageSection, error) {
	section := lineageSection{Follow: true}
	if len(raw) == 0 {
		return section, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&section); err != nil {
		return lineageSection{}, fmt.Errorf("%w: lineage section: %w", ErrConfig, err)
	}

	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return lineageSection{}, fmt.Errorf("%w: lineage section: trailing data", ErrConfig)
	}

	if err := anchoring.Validate(section.Anchors, section.Policy); err != nil {
		return lineageSection{}, fmt.Errorf("%w: lineage section: %w", ErrConfig, err)
	}

	return section, nil
}
