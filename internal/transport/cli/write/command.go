package write

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
)

var ErrUsage = errors.New("usage")

type options struct {
	repo        string
	manifest    string
	ref         string
	dateRef     string
	subjects    string
	blobDir     string
	configPath  string
	verify      string
	authorName  string
	authorEmail string
	allowDraft  bool
}

func Run(ctx context.Context, args []string, stdout io.Writer) error {
	opts, err := parse(args)
	if err != nil {
		return err
	}

	if opts, err = resolve(opts); err != nil {
		return err
	}

	return execute(ctx, opts, stdout)
}

func parse(args []string) (options, error) {
	var opts options

	set := flag.NewFlagSet("write", flag.ContinueOnError)
	set.SetOutput(io.Discard)

	for name, target := range map[string]*string{
		"repo": &opts.repo, "manifest": &opts.manifest, "ref": &opts.ref,
		"date-ref": &opts.dateRef, "subjects": &opts.subjects, "blob-dir": &opts.blobDir,
		"config": &opts.configPath, "verify": &opts.verify,
		"author-name": &opts.authorName, "author-email": &opts.authorEmail,
	} {
		set.StringVar(target, name, "", "")
	}

	set.BoolVar(&opts.allowDraft, "allow-draft", false, "")

	if err := set.Parse(args); err != nil {
		return options{}, fmt.Errorf("%w: %w", ErrUsage, err)
	}

	if set.NArg() > 0 {
		return options{}, fmt.Errorf("%w: unexpected argument", ErrUsage)
	}

	required := []struct{ name, value string }{
		{"--repo", opts.repo}, {"--manifest", opts.manifest}, {"--ref", opts.ref},
	}

	for _, field := range required {
		if field.value == "" {
			return options{}, fmt.Errorf("%w: %s is required", ErrUsage, field.name)
		}
	}

	if err := checkRef(opts.ref); err != nil {
		return options{}, err
	}

	return opts, nil
}
