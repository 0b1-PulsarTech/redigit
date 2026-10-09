package dart

import (
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestExportRule(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		file string
		want map[string]string
	}

	testCases := []testCase{
		{
			"a show list waits for each shown name, a bare export for the file's first declaration",
			"lib/app.dart",
			map[string]string{"export src/money.dart": "Money", "export src/hidden.dart": "Secret"},
		},
		{
			"an export waits for a file holding only an export",
			"lib/l10n.dart",
			map[string]string{"export src/l10n.dart": "export catalog.g.dart"},
		},
		{"an export leaving the tree waits for nothing", "lib/src/l10n.dart", map[string]string{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := map[string]string{}
			for _, edge := range edgesFrom(t, exportRule{}, library, tc.file) {
				got[edge.Dependent.Name] = edge.Provider.Name

				if edge.Reason != entities.ReasonReference {
					t.Fatalf("Reason = %v, want reference", edge.Reason)
				}
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("edges = %v, want %v", got, tc.want)
			}
		})
	}
}
