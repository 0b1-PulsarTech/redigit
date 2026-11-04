package pinrule

import (
	"context"
	"errors"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

var (
	ErrDeadPattern         = errors.New("pin matches no declaration before the cut")
	ErrSelfPin             = errors.New("pin orders a declaration after itself")
	ErrProductionAfterTest = errors.New("pin orders a production declaration after a test")
)

type Pack struct{ pins []*Rule }

var _ indexing.RulePack = Pack{}

func NewPack(pins ...*Rule) Pack { return Pack{pins: pins} }

func (Pack) Name() string { return "pins" }

func (pack Pack) Edges(
	_ context.Context,
	_ entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	var edges []entities.Edge

	for _, pin := range pack.pins {
		found, err := pin.pinEdges(decls)
		if err != nil {
			return nil, err
		}

		edges = append(edges, found...)
	}

	return edges, nil
}

func (Pack) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

func (rule *Rule) pinEdges(decls []entities.Decl) ([]entities.Edge, error) {
	var dependents, providers []entities.Decl

	for _, decl := range decls {
		if rule.decl.matches(decl.Key) {
			dependents = append(dependents, decl)
		}

		if rule.after.matches(decl.Key) {
			providers = append(providers, decl)
		}
	}

	for _, side := range []struct {
		pat   pattern
		found int
	}{{rule.decl, len(dependents)}, {rule.after, len(providers)}} {
		if side.found == 0 {
			return nil, fmt.Errorf("%w: %q", ErrDeadPattern, side.pat.text)
		}
	}

	providers = withStatements(providers, decls)
	edges := make([]entities.Edge, 0, len(dependents)*len(providers))

	for _, dependent := range dependents {
		for _, provider := range providers {
			if dependent.Key == provider.Key || sameFunction(dependent.Key, provider.Key) {
				continue
			}

			if entities.IsTestSide(provider) && !entities.IsTestSide(dependent) {
				return nil, fmt.Errorf(
					"%w: %s after %s", ErrProductionAfterTest, dependent.Key, provider.Key,
				)
			}

			edges = append(edges, entities.Edge{
				Dependent: dependent.Key, Provider: provider.Key, Reason: entities.ReasonPin,
			})
		}
	}

	if len(edges) == 0 {
		return nil, fmt.Errorf("%w: %q", ErrSelfPin, rule.decl.text)
	}

	return edges, nil
}
