package resume

import (
	"flag"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

type options struct {
	state   string
	rest    anchoring.Policy
	isAbort bool
	folds   []string
	wips    []string
}

func parseOptions(args []string) (options, error) {
	var (
		opts options
		rest string
	)

	set := flag.NewFlagSet(Verb, flag.ContinueOnError)
	set.StringVar(&opts.state, "state", "", "the state directory of the stopped run")
	set.StringVar(&rest, "rest", "", "fold or wip")
	set.BoolVar(&opts.isAbort, "abort", false, "remove the state")
	set.Func("fold", "REV", collect(&opts.folds))
	set.Func("wip", "REV", collect(&opts.wips))

	if err := set.Parse(args); err != nil {
		return options{}, fmt.Errorf("%w: %w", write.ErrUsage, err)
	}

	opts.rest = anchoring.Policy(rest)
	isRestKnown := opts.rest == "" || opts.rest == anchoring.PolicyFold ||
		opts.rest == anchoring.PolicyWIP
	isDeciding := rest != "" || len(opts.folds)+len(opts.wips) > 0

	switch {
	case opts.state == "":
		return options{}, fmt.Errorf("%w: --state is required", write.ErrUsage)
	case !isRestKnown:
		return options{}, fmt.Errorf("%w: --rest takes fold or wip", write.ErrUsage)
	case opts.isAbort == isDeciding:
		return options{}, fmt.Errorf(
			"%w: give decisions or --abort, not both or neither",
			write.ErrUsage,
		)
	}

	return opts, nil
}

func decisionsOf(opts options, state anchorstate.State) (map[string]anchoring.Policy, error) {
	out := map[string]anchoring.Policy{}

	for policy, revs := range map[anchoring.Policy][]string{
		anchoring.PolicyFold: opts.folds, anchoring.PolicyWIP: opts.wips,
	} {
		for _, rev := range revs {
			var matches []string

			for _, conflict := range state.Conflicts {
				if len(rev) > 0 && len(conflict.Rev) >= len(rev) && conflict.Rev[:len(rev)] == rev {
					matches = append(matches, conflict.Rev)
				}
			}

			if _, isTwice := out[firstOf(matches)]; len(matches) != 1 || isTwice {
				return nil, fmt.Errorf("%w: %s names %d conflicts, or one decided twice",
					write.ErrUsage, rev, len(matches))
			}

			out[matches[0]] = policy
		}
	}

	return out, nil
}

func firstOf(list []string) string {
	if len(list) == 0 {
		return ""
	}

	return list[0]
}

func collect(list *[]string) func(string) error {
	return func(rev string) error {
		*list = append(*list, rev)

		return nil
	}
}
