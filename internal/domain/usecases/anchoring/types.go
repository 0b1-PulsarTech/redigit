package anchoring

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
)

var (
	ErrProbe  = errors.New("pre-image probe failed")
	ErrAnchor = errors.New("anchor cannot be placed")
)

type Policy string

const (
	PolicyFold Policy = "fold"
	PolicyWIP  Policy = "wip"
)

type (
	Spec struct {
		Rev    string `json:"rev"`
		Kind   string `json:"kind"`
		Policy Policy `json:"policy"`
	}

	Anchor struct {
		Rev      string
		Position entities.Position
		Policy   Policy
	}

	Revision struct {
		Subject string
		Files   []FileChange
	}

	FileChange struct {
		Path, OldPath, Blob, OldBlob string
	}

	Edit struct {
		File string
		Span entities.Span
		Text string
		Keep []entities.DeclKey
	}

	Verdict struct {
		IsBuilt     bool
		IsUnchecked bool
		IsUnfit     bool
		Detail      string
	}

	Config struct {
		Inner     Delegate
		Policy    Policy
		Anchors   []Anchor
		Floor     map[entities.DeclKey]entities.Position
		Renames   lineage.Renames
		Languages []lineage.Language
		Revisions Revisions
		Finals    Finals
		Probe     Probe
		Reader    Reader
		Bound     context.Context
		Logger    *slog.Logger
	}

	Planner struct {
		cfg Config
	}
)

func New(cfg Config) Planner {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return Planner{cfg: cfg}
}

type stopPolicyError struct{}

func (stopPolicyError) Error() string { return "policy stop arrives in 24b" }

func (stopPolicyError) Is(target error) bool { return target == ErrAnchor }

func checkPolicy(policy Policy) error {
	switch policy {
	case "", PolicyFold, PolicyWIP:
		return nil
	case "stop":
		return stopPolicyError{}
	default:
		return fmt.Errorf("%w: unknown policy %q", ErrAnchor, policy)
	}
}

func Validate(specs []Spec, policy Policy) error {
	if err := checkPolicy(policy); err != nil {
		return err
	}

	seen := map[string]bool{}

	for _, spec := range specs {
		switch {
		case spec.Rev == "":
			return fmt.Errorf("%w: an anchor needs a rev", ErrAnchor)
		case spec.Kind != "fix" && spec.Kind != "perf":
			return fmt.Errorf("%w: anchor %s kind %q is not fix or perf",
				ErrAnchor, spec.Rev, spec.Kind)
		case seen[spec.Rev]:
			return fmt.Errorf("%w: anchor %s is listed twice", ErrAnchor, spec.Rev)
		}

		if err := checkPolicy(spec.Policy); err != nil {
			return err
		}

		seen[spec.Rev] = true
	}

	return nil
}

func Locate(specs []Spec, history []entities.SourceTouch) ([]Anchor, error) {
	var anchors []Anchor

	for _, spec := range specs {
		var matches []entities.SourceTouch

		for _, touch := range history {
			if strings.HasPrefix(touch.Rev, spec.Rev) {
				matches = append(matches, touch)
			}
		}

		if len(matches) != 1 {
			return nil, fmt.Errorf(
				"%w: rev %s names %d commits of the source history",
				ErrAnchor, spec.Rev, len(matches),
			)
		}

		found := Anchor{Rev: matches[0].Rev, Position: matches[0].Position, Policy: spec.Policy}

		if slices.ContainsFunc(anchors, func(one Anchor) bool { return one.Rev == found.Rev }) {
			return nil, fmt.Errorf("%w: rev %s is listed twice", ErrAnchor, spec.Rev)
		}

		anchors = append(anchors, found)
	}

	return anchors, nil
}
