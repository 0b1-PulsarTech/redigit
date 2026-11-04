package lint

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

const (
	Usage = `
  redigit lint --manifest FILE [--blob-dir DIR] [--config FILE] [--fail-over]
      Measure the shape of an emit manifest and print one JSON line per
      finding and a final {"summary":true,...} line. It reports; it never
      changes a plan. Exit 0 whenever it ran, whatever the measures, unless
      --fail-over is given (then exit 1 when any finding exists, or when no
      blob was read, since nothing was measured). A manifest with no row is
      refused. It decides nothing else. These numbers are this scanner's, not
      parity with terectek. A manifest records bytes, not lines, so lines are
      counted from the blobs in --blob-dir (handles are files at DIR/<handle>);
      without it no measure runs and the summary says so. The measure is net
      line growth: a path adds its lines now minus its lines in the last row
      that held it (never below 0), so a rewrite at equal length adds 0.
      Generated is the Code generated marker on the blob, not the directory.
      Measures (each summary entry lists what it does not see):
        source_added_over  source lines (not generated) grown by a row > 900
        generated_mixed    generated lines grown by a row > 900, row also grew source
        rows_over          lines grown by a row over every path > 850
      Not measured here (need the plan pipeline, arrive as 17b): errcheck pairs
      split, shells without tail, shells born empty, unused at birth, born as
      stubs with a gap over 20 rows. Their definitions of "shell" and "stub" are
      decided there.
      --config FILE  a redigit.json whose "lint" section may hold
                     {"sourceAdded","generatedMixed","rowAdded"} (positive ints)
`
)

var (
	ErrFailed      = errors.New("lint found shape problems")
	ErrNotMeasured = errors.New("lint measured nothing")
)

type options struct {
	manifest string
	blobDir  string
	config   string
	failOver bool
}

func Run(_ context.Context, args []string, stdout io.Writer) error {
	opts, err := parse(args)
	if err != nil {
		return err
	}

	limits, err := loadLimits(opts.config)
	if err != nil {
		return err
	}

	rows, err := readRows(opts.manifest, opts.blobDir)
	if err != nil {
		return err
	}

	report := linting.Check(rows.rows, limits)
	if !rows.measured {
		report.Measures = nil
	}

	if err = printReport(stdout, report); err != nil {
		return err
	}

	switch {
	case opts.failOver && !rows.measured:
		return fmt.Errorf("%w: no blob was read", ErrNotMeasured)
	case opts.failOver && len(report.Findings) > 0:
		return fmt.Errorf("%w: %d findings", ErrFailed, len(report.Findings))
	}

	return nil
}

func parse(args []string) (options, error) {
	var opts options

	set := flag.NewFlagSet("lint", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&opts.manifest, "manifest", "", "")
	set.StringVar(&opts.blobDir, "blob-dir", "", "")
	set.StringVar(&opts.config, "config", "", "")
	set.BoolVar(&opts.failOver, "fail-over", false, "")

	if err := set.Parse(args); err != nil {
		return options{}, fmt.Errorf("%w: %w", write.ErrUsage, err)
	}

	if set.NArg() > 0 {
		return options{}, fmt.Errorf("%w: unexpected argument", write.ErrUsage)
	}

	if opts.manifest == "" {
		return options{}, fmt.Errorf("%w: --manifest is required", write.ErrUsage)
	}

	return opts, nil
}
