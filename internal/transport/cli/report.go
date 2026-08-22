package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"slices"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// readerDTO is one declaration that reads another, dated in the SOURCE
// history's own chronology rather than in any rebuilt order.
type readerDTO struct {
	Decl   string `json:"decl"`
	At     int    `json:"at"`
	Reason string `json:"reason"`
	Unit   string `json:"unit"`
}

// reportLine is one declaration, one line. Readers carries every reader, not
// just the nearest: a declaration one neighbour reads is a candidate to defer,
// while one twelve scattered callers read is residue, and only the fan-out
// tells them apart.
type reportLine struct {
	Decl      string      `json:"decl"`
	Unit      string      `json:"unit"`
	IsTest    bool        `json:"is_test"`
	BornAt    int         `json:"born_at"`
	FirstRead *readerDTO  `json:"first_read"`
	Gap       int         `json:"gap"`
	Fanout    int         `json:"fanout"`
	Readers   []readerDTO `json:"readers"`
}

// runReport writes one JSON object per declaration, ordered by how far it sits
// ahead of its first outside reader.
//
// Positions are the SOURCE history's, not a rebuilt ordering's, because the
// question being answered is where things stand now. Birth comes from reading
// each file as it stood at each commit that touched it: a file's own first
// touch dates every declaration it ever gains to the moment it was created
// nearly empty, which is precisely the measurement that breaks a move.
func runReport(ctx context.Context, cfg bootstrap.Config, out io.Writer) error {
	return withPipeline(ctx, cfg, func(pipeline *bootstrap.Pipeline) error {
		result, err := pipeline.Graph(ctx)
		if err != nil {
			return fmt.Errorf("build graph: %w", err)
		}

		warnBlind(result)

		graph := result.Graph

		births, err := pipeline.Births(ctx)
		if err != nil {
			return fmt.Errorf("scan births: %w", err)
		}

		lines := reportLines(graph, births)

		encoder := json.NewEncoder(out)
		for _, line := range lines {
			if err = encoder.Encode(line); err != nil {
				return fmt.Errorf("write report line: %w", err)
			}
		}

		// What the scan could not read goes to stderr rather than being
		// dropped. A report that silently skipped part of the history would
		// still look complete, which is the failure mode this whole tool is
		// built around.
		if len(births.Unreadable) > 0 {
			slog.Warn(
				"some file versions could not be read",
				slog.Int("count", len(births.Unreadable)),
				slog.String("first", births.Unreadable[0]),
			)
		}

		return nil
	})
}

func reportLines(graph entities.Graph, births indexing.Births) []reportLine {
	lines := make([]reportLine, 0, graph.Len())

	for _, decl := range graph.Decls() {
		born, dated := births.At[decl.Key]
		if !dated {
			born = entities.NoPosition
		}

		readers := readersOf(graph, births, decl.Key)

		line := reportLine{
			Decl:    decl.Key.String(),
			Unit:    string(decl.Unit),
			IsTest:  decl.IsTest,
			BornAt:  int(born),
			Fanout:  len(readers),
			Readers: readers,
			Gap:     0,
		}

		if len(readers) > 0 {
			first := readers[0]
			line.FirstRead = &first
			line.Gap = first.At - int(born)
		}

		lines = append(lines, line)
	}

	slices.SortFunc(lines, func(a, b reportLine) int {
		if by := cmp.Compare(b.Gap, a.Gap); by != 0 {
			return by
		}

		return cmp.Compare(a.Decl, b.Decl)
	})

	return lines
}

// readersOf lists every reader outside the declaration's own unit, nearest
// first. Readers inside the unit are left out on purpose: counting them makes
// a package look consumed the instant its own file was created, which hides
// the real distance to whoever actually needs it.
func readersOf(
	graph entities.Graph,
	births indexing.Births,
	key entities.DeclKey,
) []readerDTO {
	edges := graph.ExternalConsumers(key)
	out := make([]readerDTO, 0, len(edges))

	for _, edge := range edges {
		at, dated := births.At[edge.Dependent]
		if !dated {
			at = entities.NoPosition
		}

		unit := ""
		if reader, known := graph.Decl(edge.Dependent); known {
			unit = string(reader.Unit)
		}

		out = append(out, readerDTO{
			Decl:   edge.Dependent.String(),
			At:     int(at),
			Reason: string(edge.Reason),
			Unit:   unit,
		})
	}

	slices.SortFunc(out, func(a, b readerDTO) int {
		if by := cmp.Compare(a.At, b.At); by != 0 {
			return by
		}

		return cmp.Compare(a.Decl, b.Decl)
	})

	return out
}
