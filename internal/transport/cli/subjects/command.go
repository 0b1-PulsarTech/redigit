package subjects

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

const (
	flagManifest = "manifest"
)

var ErrFindings = errors.New("subjects findings")

func Run(_ context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: subjects needs digest, check or carry", write.ErrUsage)
	}

	switch args[0] {
	case "digest":
		return runDigest(args[1:], stdout)
	case "check":
		return runCheck(args[1:], stdout)
	case "carry":
		return runCarry(args[1:], stdout)
	}

	return fmt.Errorf("%w: unknown subjects command %q", write.ErrUsage, args[0])
}

type requirement struct{ name, value string }

func parse(name string, args []string, bind func(*flag.FlagSet), needs ...*requirement) error {
	set := flag.NewFlagSet("subjects "+name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	bind(set)

	if err := set.Parse(args); err != nil {
		return fmt.Errorf("%w: %w", write.ErrUsage, err)
	}

	if set.NArg() > 0 {
		return fmt.Errorf("%w: unexpected argument", write.ErrUsage)
	}

	for _, need := range needs {
		if need.value == "" {
			return fmt.Errorf("%w: --%s is required", write.ErrUsage, need.name)
		}
	}

	return nil
}
