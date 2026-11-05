package report

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
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

// RunReport writes one JSON object per declaration, ordered by how far it sits
// ahead of its first outside reader.
//
// Positions are the SOURCE history's, not a rebuilt ordering's, because the
// question being answered is where things stand now. Birth comes from reading
// each file as it stood at each commit that touched it: a file's own first
// touch dates every declaration it ever gains to the moment it was created
// nearly empty, which is precisely the measurement that breaks a move.
func RunReport(
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
