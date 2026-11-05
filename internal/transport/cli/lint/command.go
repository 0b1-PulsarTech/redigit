package lint

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
	"github.com/wrapped-owls/redigit/internal/transport/cli/session"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
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
	file     *bootstrap.ConfigFile
	pipe     pipeOptions
	set      *flag.FlagSet
}

func Run(ctx context.Context, args []string, stdout io.Writer, env session.Env) error {
	opts, err := parse(args)
	if err != nil {
		return err
	}

	file, limits, err := loadConfig(opts.config)
	if err != nil {
		return err
	}

	opts.file = file

	rows, err := readRows(opts.manifest, opts.blobDir)
	if err != nil {
		return err
	}

	report := linting.Check(rows.rows, limits)
	if !rows.measured {
		report.Measures = nil
	}

	report.Measured = rows.measured

	if opts.pipe.repo != "" {
		deep, deepErr := checkStructure(ctx, opts, rows.decoded, limits, env)
		if deepErr != nil {
			return deepErr
		}

		report.Findings = append(report.Findings, deep.Findings...)
		report.Measures = append(report.Measures, deep.Measures...)
		report.Graph = deep.Graph
	}

	if err = printReport(stdout, report); err != nil {
		return err
	}

	switch {
	case opts.failOver && !rows.measured && report.Graph == nil:
		return fmt.Errorf("%w: no blob was read", ErrNotMeasured)
	case opts.failOver && linting.Counted(report.Findings) > 0:
		return fmt.Errorf(
			"%w: %d findings counted of %d", ErrFailed,
			linting.Counted(report.Findings), len(report.Findings),
		)
	}

	return nil
}

func parse(args []string) (options, error) {
	opts := options{set: flag.NewFlagSet("lint", flag.ContinueOnError)}

	opts.set.SetOutput(io.Discard)
	opts.set.StringVar(&opts.manifest, "manifest", "", "")
	opts.set.StringVar(&opts.blobDir, "blob-dir", "", "")
	opts.set.StringVar(&opts.config, "config", "", "")
	opts.set.BoolVar(&opts.failOver, "fail-over", false, "")
	registerPipeFlags(opts.set, &opts.pipe)

	if err := opts.set.Parse(args); err != nil {
		return options{}, fmt.Errorf("%w: %w", write.ErrUsage, err)
	}

	if opts.set.NArg() > 0 {
		return options{}, fmt.Errorf("%w: unexpected argument", write.ErrUsage)
	}

	if opts.manifest == "" {
		return options{}, fmt.Errorf("%w: --manifest is required", write.ErrUsage)
	}

	if err := checkPipeFlags(opts); err != nil {
		return options{}, err
	}

	return opts, nil
}
