package format

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

var ErrStrict = errors.New("formatter failures under --strict")

type options struct {
	manifest   string
	out        string
	blobDir    string
	outBlobDir string
	configPath string
	strict     bool
}

func Run(ctx context.Context, args []string, stdout io.Writer) error {
	opts, err := parse(args)
	if err != nil {
		return err
	}

	set, err := loadTools(opts.configPath)
	if err != nil {
		return err
	}

	return execute(ctx, opts, set, stdout)
}

func parse(args []string) (options, error) {
	var opts options

	set := flag.NewFlagSet("format", flag.ContinueOnError)
	set.SetOutput(io.Discard)

	required := []struct {
		name   string
		target *string
	}{
		{"manifest", &opts.manifest},
		{"out", &opts.out},
		{"blob-dir", &opts.blobDir},
		{"out-blob-dir", &opts.outBlobDir},
		{"config", &opts.configPath},
	}

	for _, field := range required {
		set.StringVar(field.target, field.name, "", "")
	}

	set.BoolVar(&opts.strict, "strict", false, "")

	if err := set.Parse(args); err != nil {
		return options{}, fmt.Errorf("%w: %w", write.ErrUsage, err)
	}

	if set.NArg() > 0 {
		return options{}, fmt.Errorf("%w: unexpected argument", write.ErrUsage)
	}

	for _, field := range required {
		if *field.target == "" {
			return options{}, fmt.Errorf("%w: --%s is required", write.ErrUsage, field.name)
		}
	}

	return opts, refuseOverwrite(opts)
}
