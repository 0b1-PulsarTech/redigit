package lint

import (
	"bytes"
	"encoding/json"
	"errors"
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
				Rows: 3, Measures: []linting.Measure{{Name: "m", Excluded: []string{"x"}}},
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
