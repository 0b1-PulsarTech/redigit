package auditing

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestAuditAppendsGateMeasures(t *testing.T) {
	t.Parallel()

	passing := entities.Measure{
		Name: "di boot windows", Value: 4, Total: 4, Excluded: []string{"dynamic registration"},
	}
	failing := entities.Measure{
		Name: "di boot windows", Value: 1, Total: 4, Excluded: []string{"dynamic registration"},
	}
	blind := entities.Measure{Name: "di boot windows", Value: 4, Total: 4}
	errBlind := apperr.ErrIncomplete

	testCases := []struct {
		name    string
		gates   []entities.Measure
		wantLen int
		wantErr error
	}{
		{"no gate keeps the integrity measures", nil, 3, nil},
		{"one passing gate is appended last", []entities.Measure{passing}, 4, nil},
		{"one failing gate is reported as measured", []entities.Measure{failing}, 4, nil},
		{"two gates keep their order", []entities.Measure{passing, failing}, 5, nil},
		{"a gate that names no blind spot is refused", []entities.Measure{blind}, 0, errBlind},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph, order, _ := wiring()
			ctrl := gomock.NewController(t)

			var gates []Gate

			for _, measure := range testCase.gates {
				gate := NewMockGate(ctrl)
				gate.EXPECT().Measure(graph, order).Return(measure)
				gates = append(gates, gate)
			}

			result, err := New(Config{Gates: gates}).Audit(graph, order)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Audit() = %v, want %v", err, testCase.wantErr)
			}

			if err != nil {
				return
			}

			if got := len(result.Report.Integrity); got != testCase.wantLen {
				t.Fatalf("integrity measures = %d, want %d", got, testCase.wantLen)
			}

			for at, measure := range testCase.gates {
				if got := result.Report.Integrity[3+at]; !reflect.DeepEqual(got, measure) {
					t.Fatalf("gate %d measure = %+v, want %+v", at, got, measure)
				}
			}
		})
	}
}

func TestAuditWithoutGatesIsUnchanged(t *testing.T) {
	t.Parallel()

	graph, order, _ := wiring()

	golden, err := os.ReadFile("testdata/audit_base.golden.json")
	if err != nil {
		t.Fatalf("ReadFile() = %v", err)
	}

	testCases := []struct {
		name string
		cfg  Config
	}{
		{"a config with no gates", Config{}},
		{"a config with an empty gate list", Config{Gates: []Gate{}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result, auditErr := New(testCase.cfg).Audit(graph, order)
			if auditErr != nil {
				t.Fatalf("Audit() = %v", auditErr)
			}

			raw, marshalErr := json.MarshalIndent(result, "", "  ")
			if marshalErr != nil {
				t.Fatalf("MarshalIndent() = %v", marshalErr)
			}

			if got := append(raw, '\n'); !bytes.Equal(got, golden) {
				t.Fatalf("audit differs from the report before gates existed:\n%s\nwant:\n%s",
					got, golden)
			}
		})
	}
}
