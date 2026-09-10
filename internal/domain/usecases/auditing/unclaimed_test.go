package auditing

import "testing"

func TestAuditCarriesUnclaimed(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		unclaimed []Unclaimed
		wantLen   int
	}{
		{
			name:      "every path was claimed by some language pack",
			unclaimed: nil,
			wantLen:   0,
		},
		{
			name: "a path no language pack claims travels through",
			unclaimed: []Unclaimed{
				{File: "engines/wwebjs/src/index.ts"},
				{File: "database/atlas.hcl"},
			},
			wantLen: 2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph, order, _ := wiring()

			result, err := New(Config{Unclaimed: testCase.unclaimed}).Audit(graph, order)
			if err != nil {
				t.Fatalf("audit: %v", err)
			}

			if len(result.Unclaimed) != testCase.wantLen {
				t.Fatalf("unclaimed = %d, want %d", len(result.Unclaimed), testCase.wantLen)
			}

			for i, want := range testCase.unclaimed {
				if result.Unclaimed[i].File != want.File {
					t.Fatalf(
						"unclaimed[%d].File = %q, want %q",
						i,
						result.Unclaimed[i].File,
						want.File,
					)
				}
			}
		})
	}
}
