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
	}

	measureLine struct {
		Name       string   `json:"name"`
		Definition string   `json:"definition"`
		Limit      int      `json:"limit"`
		Count      int      `json:"count"`
		Excluded   []string `json:"excluded"`
	}

	summaryLine struct {
		Summary  bool          `json:"summary"`
		Rows     int           `json:"rows"`
		Measured bool          `json:"measured"`
		Note     string        `json:"note,omitempty"`
		Findings int           `json:"findings"`
		Measures []measureLine `json:"measures"`
		Deferred []string      `json:"deferred"`
	}
)

func printReport(out io.Writer, report linting.Report) error {
	measured := len(report.Measures) > 0
	encoder := json.NewEncoder(out)

	for _, finding := range report.Findings {
		if err := encoder.Encode(findingLine(finding)); err != nil {
			return fmt.Errorf("write finding: %w", err)
		}
	}

	summary := summaryLine{
		Summary: true, Rows: report.Rows, Measured: measured, Findings: len(report.Findings),
		Measures: []measureLine{}, Deferred: linting.Deferred(),
	}

	if !measured {
		summary.Note = "no blob was read (--blob-dir missing, or no row holds a blob): no line measure ran"
	}

	for _, measure := range report.Measures {
		summary.Measures = append(summary.Measures, measureLine(measure))
	}

	if err := encoder.Encode(summary); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	return nil
}
