package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/ordering"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

type (
	lineageSection struct {
		follow  bool
		anchors json.RawMessage
	}

	lineageScan func(context.Context, *Config, stageEnv, lineage.Config) (lineage.Result, error)

	lineageOrderer struct {
		ordering.Orderer

		result lineage.Result
	}
)

func lineageHook(ctx context.Context, cfg *Config, env stageEnv, built *stages) error {
	return lineageStage(ctx, cfg, env, built, scanHistory)
}

func lineageStage(
	ctx context.Context,
	cfg *Config,
	env stageEnv,
	built *stages,
	scan lineageScan,
) error {
	// Decoded in every mode, so a bad section fails before any indexing starts.
	section, err := decodeLineageSection(cfg.Sections.Lineage)
	if err != nil {
		return err
	}

	if cfg.Mode != ModeLineage {
		return nil
	}

	result, err := scan(ctx, cfg, env, lineage.Config{
		Languages: lineageLanguages(env.specific), Follow: section.follow,
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

	return nil
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
	if closeErr := revisions.Close(); closeErr != nil {
		err = errors.Join(err, closeErr)
	}

	return result, err
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
	section := lineageSection{follow: true}
	if len(raw) == 0 {
		return section, nil
	}

	var shape struct {
		Follow  *bool           `json:"follow"`
		Anchors json.RawMessage `json:"anchors"`
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&shape); err != nil {
		return lineageSection{}, fmt.Errorf("%w: lineage section: %w", ErrConfig, err)
	}

	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return lineageSection{}, fmt.Errorf("%w: lineage section: trailing data", ErrConfig)
	}

	if shape.Follow != nil {
		section.follow = *shape.Follow
	}

	section.anchors = shape.Anchors

	return section, nil
}
