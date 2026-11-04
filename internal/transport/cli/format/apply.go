package format

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
)

type (
	summary struct {
		Blobs          int `json:"blobs"`
		Handles        int `json:"handles"`
		Claimed        int `json:"claimed"`
		Changed        int `json:"changed"`
		Unchanged      int `json:"unchanged"`
		Failed         int `json:"failed"`
		PassedThrough  int `json:"passed_through"`
		FinalLeftAlone int `json:"final_left_alone"`
	}

	settled struct {
		handles  map[unit]string
		contents map[string][]byte
		failures []error
		counts   summary
	}
)

func execute(ctx context.Context, opts options, set tools, stdout io.Writer) error {
	rows, err := readRows(opts.manifest)
	if err != nil {
		return err
	}

	finals := finalSpots(rows)
	work := collect(rows, finals, set)

	verdicts, err := formatUnits(ctx, set.engine, opts.blobDir, work.units)
	if err != nil {
		return err
	}

	if err = ctx.Err(); err != nil {
		return fmt.Errorf("format blobs: %w", err)
	}

	done := settle(work, verdicts)
	for _, failure := range done.failures {
		slog.Warn("blob left raw", slog.String("detail", failure.Error()))
	}

	if opts.strict && len(done.failures) > 0 {
		return fmt.Errorf(
			"%w: %d blobs, first: %w", ErrStrict, len(done.failures), done.failures[0],
		)
	}

	rewrite(rows, finals, done.handles)

	if err = publish(opts, rows, done.contents); err != nil {
		return err
	}

	if err = json.NewEncoder(stdout).Encode(done.counts); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	return nil
}

func settle(work plan, verdicts []verdict) settled {
	done := settled{
		handles: map[unit]string{}, contents: map[string][]byte{},
		counts: summary{
			Blobs: work.blobs, Handles: work.handles, FinalLeftAlone: work.finals,
			PassedThrough: work.unclaimed, Claimed: len(work.units),
		},
	}

	for index, item := range work.units {
		outcome := verdicts[index]

		switch {
		case outcome.failure != nil:
			done.counts.Failed++
			done.failures = append(done.failures, outcome.failure)
		case bytes.Equal(outcome.content, outcome.raw):
			done.counts.Unchanged++
		default:
			sum := sha256.Sum256(outcome.content)
			fresh := hex.EncodeToString(sum[:])
			done.handles[item], done.contents[fresh] = fresh, outcome.content
			done.counts.Changed++
		}
	}

	return done
}

func rewrite(rows []manifest.Row, finals map[spot]struct{}, handles map[unit]string) {
	for rowIndex := range rows {
		for blobIndex := range rows[rowIndex].Blobs {
			blob := &rows[rowIndex].Blobs[blobIndex]
			if _, last := finals[spot{rowIndex, blobIndex}]; last || !regular(*blob) {
				continue
			}

			if fresh, found := handles[unit{handle: blob.Handle, path: blob.Path}]; found {
				blob.Handle = fresh
			}
		}
	}
}
