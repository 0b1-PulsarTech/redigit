package parity

import (
	"bytes"
	"fmt"
	"iter"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type Edge struct {
	Dependent string
	Provider  string
	Reason    entities.EdgeReason
}

const edgeFields = 3

type rawEdge struct{ dependent, provider, reason []byte }

func splitEdge(text []byte) (rawEdge, error) {
	if fields := bytes.Count(text, []byte{'\t'}) + 1; fields != edgeFields {
		return rawEdge{}, fmt.Errorf(
			"%w: edge has %d fields, want %d",
			ErrMalformed, fields, edgeFields,
		)
	}

	dependent, rest, _ := bytes.Cut(text, []byte{'\t'})
	provider, reason, _ := bytes.Cut(rest, []byte{'\t'})

	return rawEdge{dependent: dependent, provider: provider, reason: reason}, nil
}

func (raw rawEdge) edge() Edge {
	return Edge{
		Dependent: string(raw.dependent),
		Provider:  string(raw.provider),
		Reason:    entities.EdgeReason(raw.reason),
	}
}

func (raw rawEdge) isHard() bool {
	if bytes.Equal(raw.dependent, raw.provider) {
		return false
	}

	if string(raw.reason) == string(entities.ReasonImplements) {
		return implementsHard(raw.edge())
	}

	return entities.IsHard(entities.EdgeReason(raw.reason))
}

func StreamEdges(path string) iter.Seq2[Edge, error] {
	return streamEdges(openFixture, path)
}

func streamEdges(open fixtureOpener, path string) iter.Seq2[Edge, error] {
	return func(yield func(Edge, error) bool) {
		isStopped := false

		err := forEachLineOpen(open, path, func(text []byte) error {
			raw, decodeErr := splitEdge(text)
			if decodeErr != nil {
				return decodeErr
			}

			if !yield(raw.edge(), nil) {
				isStopped = true

				return errStop
			}

			return nil
		})
		// After a false yield the iterator must not yield again (it would panic), so a close error is dropped on purpose.
		if err != nil && !isStopped {
			yield(Edge{}, err)
		}
	}
}

func LoadHardEdges(path string) ([]Edge, error) {
	// Endpoints are interned: a few thousand keys repeat across hundreds of thousands of edges.
	interned := make(map[string]string)
	intern := func(key []byte) string {
		if known, found := interned[string(key)]; found {
			return known
		}

		cloned := string(key)
		interned[cloned] = cloned

		return cloned
	}

	var edges []Edge

	err := forEachLine(path, func(text []byte) error {
		raw, splitErr := splitEdge(text)
		if splitErr != nil {
			return splitErr
		}

		// Filter on bytes so the dropped soft and self edges allocate nothing.
		if !raw.isHard() {
			return nil
		}

		edges = append(edges, Edge{
			Dependent: intern(raw.dependent),
			Provider:  intern(raw.provider),
			Reason:    entities.EdgeReason(intern(raw.reason)),
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	return edges, nil
}

func implementsHard(edge Edge) bool {
	// engine.py _impl_hard: a non-mock function, statement or method needs the methods it converts to.
	dependentPath, _, _ := strings.Cut(edge.Dependent, ":")
	if !strings.Contains(edge.Provider, ":method:") || strings.Contains(dependentPath, "mock") {
		return false
	}

	return strings.Contains(edge.Dependent, ":func:") ||
		strings.Contains(edge.Dependent, ":stmt:") ||
		strings.Contains(edge.Dependent, ":method:")
}
