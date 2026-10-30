package verify

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

const (
	Usage = `
  redigit verify [--repo PATH] [--ref REF] [flags]
      Build every commit of REF (default HEAD) and report whether it is
      green: per module, the language's steps in order (Go: go build then
      go vet, with -tags). Prints one JSON line per commit and module and a
      final {"summary":true,...} line. Exit 1 when a commit that is not
      carrying a "[wip] " marker (as redigit write places it, after
      "type(scope): ") fails; wip failures are counted but tolerated. A
      commit with no module is reported "empty", never "pass".
      --from N --to N  1-based positions to cover (default: all); --from past
                       the last commit is a usage error. A run that checks
                       no module exits 1.
      --stride N       check every Nth commit, always including the tip
      --parallel N     checkout slots built at once (default 2, at most the
                       number of CPUs)
      --tags a,b       build tags for the Go steps
      --exclude d,e    module directories to skip (prefix match)
      --cache-dir DIR  slots, outcomes and scratch (default: user cache)
      --config FILE    a redigit.json whose "verify" section may hold
                       {"tags","parallel","stride","exclude","languages":
                       {NAME:{"marker","version":[argv],"env":[NAMES],
                       "steps":[{"cmd":[argv],"tolerate":[lines],
                       "timeout":"5m"}]}}} (flags win; a step passes
                       on exit 1 when every output line is a tolerate
                       entry; env names enter the cache key). A
                       config that redefines languages.go cannot also
                       carry tags or --tags: write them into its steps.
`
)

var (
	ErrUsage  = write.ErrUsage
	ErrFailed = errors.New("verification failed")
)

type options struct {
	repo       string
	ref        string
	configPath string
	cacheDir   string
	tags       string
	exclude    string
	from       int
	to         int
	stride     int
	parallel   int
	provided   map[string]bool
}

func Run(ctx context.Context, args []string, stdout io.Writer) error {
	return run(ctx, args, stdout, assemble)
}

func parse(args []string) (options, error) {
	opts := options{provided: map[string]bool{}}

	set := flag.NewFlagSet("verify", flag.ContinueOnError)
	set.SetOutput(io.Discard)

	for name, target := range map[string]*string{
		"repo": &opts.repo, "ref": &opts.ref, "config": &opts.configPath,
		"cache-dir": &opts.cacheDir, "tags": &opts.tags, "exclude": &opts.exclude,
	} {
		set.StringVar(target, name, "", "")
	}

	for name, target := range map[string]*int{
		"from": &opts.from, "to": &opts.to, "stride": &opts.stride, "parallel": &opts.parallel,
	} {
		set.IntVar(target, name, 0, "")
	}

	if err := set.Parse(args); err != nil {
		return options{}, fmt.Errorf("%w: %w", ErrUsage, err)
	}

	if set.NArg() > 0 {
		return options{}, fmt.Errorf("%w: unexpected argument", ErrUsage)
	}

	set.Visit(func(visited *flag.Flag) { opts.provided[visited.Name] = true })

	return withDefaults(opts)
}

func withDefaults(opts options) (options, error) {
	const (
		defaultStride   = 1
		defaultParallel = 2
	)

	if !opts.provided["repo"] {
		opts.repo = "."
	}

	if !opts.provided["ref"] {
		opts.ref = "HEAD"
	}

	if !opts.provided["stride"] {
		opts.stride = defaultStride
	}

	if !opts.provided["parallel"] {
		opts.parallel = min(defaultParallel, runtime.NumCPU())
	}

	switch {
	case opts.repo == "":
		return options{}, fmt.Errorf("%w: --repo cannot be empty", ErrUsage)
	case strings.HasPrefix(opts.ref, "-"):
		return options{}, fmt.Errorf("%w: --ref %q may not start with a dash", ErrUsage, opts.ref)
	case opts.stride < 1 || opts.parallel < 1:
		return options{}, fmt.Errorf("%w: --stride and --parallel must be at least 1", ErrUsage)
	case opts.parallel > runtime.NumCPU():
		return options{}, fmt.Errorf("%w: --parallel above the %d cpus", ErrUsage, runtime.NumCPU())
	case opts.from < 0 || opts.to < 0:
		return options{}, fmt.Errorf("%w: --from and --to cannot be negative", ErrUsage)
	case hasRootExclude(splitList(opts.exclude)):
		return options{}, fmt.Errorf("%w: --exclude cannot name the repository root", ErrUsage)
	case opts.to > 0 && opts.from > opts.to:
		return options{}, fmt.Errorf("%w: --from is after --to", ErrUsage)
	}

	return opts, nil
}
