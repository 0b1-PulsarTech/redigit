package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/transport/cli/session"
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
	Decl string `json:"decl"`
	Unit string `json:"unit"`
	// IsTest marks a test FUNCTION. InTestFile marks anything living in a
	// test file, which is a wider and more dangerous class: a stub method on
	// a fake is not a test function, has no subject to follow, and is read
	// only by its own package. Judging it by an external reader moves it away
	// from the tests that use it.
	IsTest     bool       `json:"is_test"`
	InTestFile bool       `json:"in_test_file"`
	BornAt     int        `json:"born_at"`
	FirstRead  *readerDTO `json:"first_read"`
	Gap        int        `json:"gap"`
	Fanout     int        `json:"fanout"`
	// InsideFanout counts readers in the declaration's OWN unit, and
	// TestFanout counts readers that are tests, wherever they live. Fanout
	// alone cannot tell a declaration nothing reads from one its own package
	// uses constantly, and the difference is what separates an ordering
	// question from a declaration that maybe should not exist.
	InsideFanout int `json:"inside_fanout"`
	TestFanout   int `json:"test_fanout"`
	// Subject is what a test exercises, and it is the ONLY thing a test may
	// be placed by. A test has no meaningful reader: following first_read for
	// one moves it away from the function it covers and leaves that function
	// untested at the position it is born. Non-test rows carry null here.
	// InInterface marks a declaration whose type satisfies some interface. It
	// changes what fanout MEANS: a method nobody references may still be
	// required, because removing it stops its type from satisfying the
	// interface, and the compiler says so at a conversion the file does not
	// mention. Zero readers there is the opposite of safe to move.
	InInterface bool `json:"in_interface"`
	// GroupSize is how many declarations share this one's block. Greater than
	// one means the declaration is part of an enum or a bound group and does
	// not travel alone, whatever its own reader count says.
	GroupSize int        `json:"group_size"`
	Subject   *readerDTO `json:"subject"`
	// SubjectGap is how far after its subject a test was born. Negative means
	// the test is born BEFORE what it exercises, which cannot compile.
	SubjectGap int         `json:"subject_gap"`
	Readers    []readerDTO `json:"readers"`
}

// runReport writes one JSON object per declaration, ordered by how far it sits
// ahead of its first outside reader.
//
// Positions are the SOURCE history's, not a rebuilt ordering's, because the
// question being answered is where things stand now. Birth comes from reading
// each file as it stood at each commit that touched it: a file's own first
// touch dates every declaration it ever gains to the moment it was created
// nearly empty, which is precisely the measurement that breaks a move.
func runReport(
	ctx context.Context,
	cfg bootstrap.Config,
	out io.Writer,
	start session.Starter,
) error {
	//nolint:wrapcheck
	return session.WithPipeline(ctx, cfg, start, func(pl session.Pipeline) error {
		result, err := pl.Graph(ctx)
		if err != nil {
			return fmt.Errorf("build graph: %w", err)
		}

		session.WarnShortfalls(result)

		graph := result.Graph

		births, err := pl.Births(ctx)
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
	subjects := subjectsOf(graph, births)
	satisfying := satisfyingTypes(graph)

	for _, decl := range graph.Decls() {
		born, dated := births.At[decl.Key]
		if !dated {
			born = entities.NoPosition
		}

		readers := readersOf(graph, births, decl.Key)
		inside, byTests := insideAndTestFanout(graph, decl)

		line := reportLine{
			Decl:         decl.Key.String(),
			Unit:         string(decl.Unit),
			IsTest:       decl.IsTest,
			InTestFile:   strings.HasSuffix(decl.Key.File, "_test.go"),
			BornAt:       int(born),
			Fanout:       len(readers),
			InsideFanout: inside,
			TestFanout:   byTests,
			InInterface:  inInterface(satisfying, decl),
			GroupSize:    decl.GroupSize,
			Readers:      readers,
			Gap:          0,
		}

		if len(readers) > 0 {
			first := readers[0]
			line.FirstRead = &first
			line.Gap = first.At - int(born)
		}

		if subject, anchored := subjects[decl.Key]; anchored {
			line.Subject = &subject
			line.SubjectGap = int(born) - subject.At
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

// insideAndTestFanout counts, separately, the readers in a declaration's own
// unit and the readers that are tests. A declaration with no reader of any
// kind is a different finding from one its own package leans on, and neither
// is visible from the external count alone.
func insideAndTestFanout(graph entities.Graph, decl entities.Decl) (inside, byTests int) {
	seen := map[entities.DeclKey]bool{}

	for _, edge := range graph.Consumers(decl.Key) {
		if seen[edge.Dependent] {
			continue
		}

		seen[edge.Dependent] = true

		reader, known := graph.Decl(edge.Dependent)
		if !known {
			continue
		}

		if reader.Unit == decl.Unit {
			inside++
		}

		if reader.IsTest {
			byTests++
		}
	}

	return inside, byTests
}

// subjectsOf maps a test declaration to the declaration it exercises, from the
// anchors a language pack produced. The link exists because nothing else
// establishes it: a test names its subject by convention, not by a reference a
// resolver can follow, and pairing on the FILE instead dates every test in it
// to whatever the file was born with.
func subjectsOf(graph entities.Graph, births indexing.Births) map[entities.DeclKey]readerDTO {
	out := map[entities.DeclKey]readerDTO{}

	for _, anchor := range graph.Anchors() {
		if anchor.Reason != entities.ReasonTestOf {
			continue
		}

		at, dated := births.At[anchor.To]
		if !dated {
			at = entities.NoPosition
		}

		unit := ""
		if subject, known := graph.Decl(anchor.To); known {
			unit = string(subject.Unit)
		}

		out[anchor.Decl] = readerDTO{
			Decl:   anchor.To.String(),
			At:     int(at),
			Reason: string(anchor.Reason),
			Unit:   unit,
		}
	}

	return out
}

// satisfyingTypes names every type that satisfies some interface, keyed by its
// unit and name so a method can be matched to its receiver even when the type
// is declared in another file of the same package.
func satisfyingTypes(graph entities.Graph) map[string]bool {
	out := map[string]bool{}

	for _, edge := range graph.Edges() {
		if edge.Reason != entities.ReasonImplements {
			continue
		}

		if decl, known := graph.Decl(edge.Dependent); known {
			out[string(decl.Unit)+"."+decl.Key.Name] = true
		}
	}

	return out
}

// inInterface reports whether a declaration is bound to an interface: the type
// itself, or a method of a type that satisfies one.
func inInterface(satisfying map[string]bool, decl entities.Decl) bool {
	name := decl.Key.Name
	if decl.Key.Kind == entities.KindMethod {
		receiver, _, found := strings.Cut(name, ".")
		if !found {
			return false
		}

		name = receiver
	}

	return satisfying[string(decl.Unit)+"."+name]
}
