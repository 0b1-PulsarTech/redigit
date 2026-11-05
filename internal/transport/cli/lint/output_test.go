package lint

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
)

type failWriter struct{ after int }

var errBroken = errors.New("broken pipe")

func (writer *failWriter) Write(payload []byte) (int, error) {
	if writer.after == 0 {
		return 0, errBroken
	}

	writer.after--

	return len(payload), nil
}

func TestPrintReportReportsAFailingWriter(t *testing.T) {
	t.Parallel()

	report := linting.Report{Findings: []linting.Finding{{Measure: "m"}}}

	testCases := []struct {
		name  string
		after int
	}{
		{name: "the finding line fails", after: 0},
		{name: "the summary line fails", after: 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := printReport(&failWriter{after: testCase.after}, report)
			if !errors.Is(err, errBroken) {
				t.Fatalf("printReport() err = %v", err)
			}
		})
	}
}

func TestPrintReportSaysWhenNothingWasMeasured(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		report       linting.Report
		wantMeasured bool
		wantNote     string
	}{
		{
			name:     "no measure leaves the note that says no line measure ran",
			report:   linting.Report{Rows: 3},
			wantNote: "no blob was read (--blob-dir missing, or no row holds a blob): no line measure ran",
		},
		{
			name: "a measure leaves no note",
			report: linting.Report{
				Rows: 3, Measured: true,
				Measures: []linting.Measure{{Name: "m", Excluded: []string{"x"}}},
			},
			wantMeasured: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			if err := printReport(&out, testCase.report); err != nil {
				t.Fatalf("printReport() err = %v", err)
			}

			var summary summaryOut
			if err := json.Unmarshal(out.Bytes(), &summary); err != nil {
				t.Fatalf("summary %q: %v", out.String(), err)
			}

			if summary.Measured != testCase.wantMeasured || summary.Note != testCase.wantNote ||
				summary.Rows != 3 {
				t.Fatalf("summary = %+v", summary)
			}
		})
	}
}

func TestGraphNoteSaysWhatItRead(t *testing.T) {
	t.Parallel()

	got := graphNote(&linting.GraphTier{Commit: "abc"})

	for _, want := range []string{"the graph read holds no cut function", "is refused"} {
		if !strings.Contains(got, want) {
			t.Fatalf("graphNote() = %q, want it to say %q", got, want)
		}
	}
}

func TestPrintReportSaysWhichTierRanAndCarriesTheDetail(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		report       linting.Report
		wantGraph    bool
		wantNote     bool
		wantDeferred int
	}{
		{
			name:         "no graph tier keeps the legacy deferred list",
			report:       linting.Report{Rows: 3},
			wantDeferred: 5,
		},
		{
			name: "a graph tier with cut functions defers nothing",
			report: linting.Report{Rows: 3, Graph: &linting.GraphTier{
				Commit: "abc", CutFunctions: 4,
			}},
			wantGraph: true,
		},
		{
			name:      "a graph tier with none says so",
			report:    linting.Report{Rows: 3, Graph: &linting.GraphTier{Commit: "abc"}},
			wantGraph: true, wantNote: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			testCase.report.Findings = []linting.Finding{{Measure: "m", Detail: "why"}}
			testCase.report.Measures = []linting.Measure{{Name: "m", Detail: "how many"}}

			var out bytes.Buffer
			if err := printReport(&out, testCase.report); err != nil {
				t.Fatalf("printReport() err = %v", err)
			}

			lines := strings.Split(strings.TrimSpace(out.String()), "\n")

			var (
				finding struct{ Detail string }
				summary struct {
					Graph *struct {
						Ran  bool
						Note string
					}
					Measures []struct{ Detail string }
					Deferred []string
				}
			)

			if err := json.Unmarshal([]byte(lines[0]), &finding); err != nil {
				t.Fatalf("finding %q: %v", lines[0], err)
			}

			if err := json.Unmarshal([]byte(lines[1]), &summary); err != nil {
				t.Fatalf("summary %q: %v", lines[1], err)
			}

			if finding.Detail != "why" || summary.Measures[0].Detail != "how many" ||
				(summary.Graph != nil && summary.Graph.Ran) != testCase.wantGraph ||
				(summary.Graph != nil && summary.Graph.Note != "") != testCase.wantNote ||
				len(summary.Deferred) != testCase.wantDeferred {
				t.Fatalf("finding = %+v, summary = %+v", finding, summary)
			}
		})
	}
}
