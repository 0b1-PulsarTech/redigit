package lint

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
)

type (
	findingLine struct {
		Measure  string `json:"measure"`
		Position int    `json:"position"`
		Subject  string `json:"subject"`
		Value    int    `json:"value"`
		Limit    int    `json:"limit"`
		Detail   string `json:"detail,omitempty"`

		NotCounted bool `json:"notCounted,omitempty"`
	}

	measureLine struct {
		Name       string   `json:"name"`
		Definition string   `json:"definition"`
		Limit      int      `json:"limit"`
		Count      int      `json:"count"`
		Detail     string   `json:"detail,omitempty"`
		Excluded   []string `json:"excluded"`
	}

	graphLine struct {
		Ran          bool   `json:"ran"`
		Commit       string `json:"commit"`
		CutFunctions int    `json:"cutFunctions"`
		Note         string `json:"note,omitempty"`
	}

	summaryLine struct {
		Summary  bool          `json:"summary"`
		Rows     int           `json:"rows"`
		Measured bool          `json:"measured"`
		Note     string        `json:"note,omitempty"`
		Findings int           `json:"findings"`
		Graph    *graphLine    `json:"graph,omitempty"`
		Measures []measureLine `json:"measures"`
		Deferred []string      `json:"deferred"`
	}
)

func printReport(out io.Writer, report linting.Report) error {
	encoder := json.NewEncoder(out)

	for _, finding := range report.Findings {
		if err := encoder.Encode(findingLine(finding)); err != nil {
			return fmt.Errorf("write finding: %w", err)
		}
	}

	summary := summaryLine{
		Summary: true, Rows: report.Rows, Measured: report.Measured, Findings: len(report.Findings),
		Measures: []measureLine{}, Deferred: linting.Deferred(),
	}

	if !report.Measured {
		summary.Note = "no blob was read (--blob-dir missing, or no row holds a blob): no line measure ran"
	}

	if report.Graph != nil {
		summary.Graph = &graphLine{
			Ran: true, Commit: report.Graph.Commit, CutFunctions: report.Graph.CutFunctions,
			Note: graphNote(report.Graph),
		}
		summary.Deferred = []string{}
	}

	for _, measure := range report.Measures {
		summary.Measures = append(summary.Measures, measureLine(measure))
	}

	if err := encoder.Encode(summary); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	return nil
}

func graphNote(tier *linting.GraphTier) string {
	if tier.CutFunctions == 0 {
		return "the graph read holds no cut function: the three function-shape measures have " +
			"nothing to read (a repository with no Go slicer is expected to land here); a " +
			"manifest that places no piece of a cut function the graph holds is refused"
	}

	return ""
}
