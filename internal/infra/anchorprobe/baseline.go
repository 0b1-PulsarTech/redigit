package anchorprobe

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

type (
	baselines struct {
		mutex sync.Mutex
		known map[string]conclusion
	}

	conclusion struct {
		detail      string
		step        string
		tail        string
		isFailed    bool
		isUnchecked bool
	}
)

func (probe *Probe) baseline(ctx context.Context, dir string, units []verifying.Unit) error {
	for _, unit := range units {
		if _, isKnown := probe.lookup(unit); isKnown {
			continue
		}

		found, err := probe.run(ctx, dir, unit)
		if err != nil {
			return err
		}

		probe.healthy.mutex.Lock()

		if probe.healthy.known == nil {
			probe.healthy.known = map[string]conclusion{}
		}

		probe.healthy.known[unit.Dir] = found
		probe.healthy.mutex.Unlock()
	}

	return nil
}

func (probe *Probe) lookup(unit verifying.Unit) (conclusion, bool) {
	probe.healthy.mutex.Lock()
	defer probe.healthy.mutex.Unlock()

	found, isKnown := probe.healthy.known[unit.Dir]

	return found, isKnown
}

func (probe *Probe) broken(unit verifying.Unit) (conclusion, bool) {
	found, _ := probe.lookup(unit)

	return found, found.isFailed || found.isUnchecked
}

func (probe *Probe) run(
	ctx context.Context,
	dir string,
	unit verifying.Unit,
) (conclusion, error) {
	outcome, err := probe.cfg.Builder.Check(ctx, dir, unit)

	switch {
	case ctx.Err() != nil:
		return conclusion{}, fmt.Errorf("check %s: %w", unit.Dir, ctx.Err())
	case err != nil:
		return conclusion{detail: fmt.Sprintf("%s: %v", unit.Dir, err), isUnchecked: true}, nil
	case !outcome.IsFailed:
		return conclusion{}, nil
	}

	return conclusion{
		detail:      fmt.Sprintf("%s: %s: %s", unit.Dir, outcome.Step, diagnostic(outcome.Tail)),
		step:        outcome.Step,
		tail:        outcome.Tail,
		isFailed:    !outcome.IsTimeout,
		isUnchecked: outcome.IsTimeout,
	}, nil
}

func (probe *Probe) trial(
	ctx context.Context,
	dir string,
	unit verifying.Unit,
) (conclusion, error) {
	before, isBroken := probe.broken(unit)
	if isBroken && before.isUnchecked {
		return unproven(unit, before), nil
	}

	after, err := probe.run(ctx, dir, unit)
	if err != nil || !isBroken {
		return after, err
	}

	if after.isFailed {
		if fresh := newLines(before.tail, after.tail, dir); len(fresh) > 0 {
			reason := diagnostic(strings.Join(fresh, "\n"))

			return conclusion{
				detail:   fmt.Sprintf("%s: %s: %s", unit.Dir, after.step, reason),
				isFailed: true,
			}, nil
		}
	}

	return unproven(unit, before), nil
}

func unproven(unit verifying.Unit, before conclusion) conclusion {
	return conclusion{
		detail:      unit.Dir + ": fails before any edit: " + before.detail,
		isUnchecked: true,
	}
}

func newLines(before, after, dir string) []string {
	place := regexp.MustCompile(`(\.\w+):\d+(:\d+)?:`)
	known := map[string]bool{}

	for _, line := range normalised(before, dir) {
		known[place.ReplaceAllString(line, "$1:")] = true
	}

	var fresh []string

	for _, line := range normalised(after, dir) {
		if !known[place.ReplaceAllString(line, "$1:")] {
			fresh = append(fresh, line)
		}
	}

	return fresh
}

func normalised(tail, dir string) []string {
	work := regexp.MustCompile(`go-build[0-9]+`)
	tail = work.ReplaceAllString(strings.ReplaceAll(tail, dir, "<tree>"), "go-build")

	var lines []string

	for line := range strings.Lines(tail) {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

func diagnostic(tail string) string {
	first := ""

	for line := range strings.Lines(tail) {
		line = strings.TrimSpace(line)

		switch {
		case line == "":
		case !strings.HasPrefix(line, "#"):
			return line
		case first == "":
			first = line
		}
	}

	return first
}
