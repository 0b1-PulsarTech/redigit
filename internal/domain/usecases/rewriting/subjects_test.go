package rewriting

import (
	"errors"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestMessages(t *testing.T) {
	t.Parallel()

	draft := entities.DraftMarker + "feat: x"

	testCases := []struct {
		name       string
		rows       []Row
		subjects   map[string]string
		allowDraft bool
		want       []string
		wantErr    error
	}{
		{
			"override wins and missing key keeps the row",
			[]Row{{Position: 0, Subject: "a"}, {Position: 1, Subject: "b"}},
			map[string]string{"0": "override"},
			false,
			[]string{"override\n", "b\n"},
			nil,
		},
		{
			"trailing newlines collapse to one",
			[]Row{{Position: 0, Subject: "a\n\n\n"}},
			nil, false,
			[]string{"a\n"},
			nil,
		},
		{
			"draft refused",
			[]Row{{Position: 0, Subject: draft}},
			nil,
			false,
			nil,
			apperr.ErrIncomplete,
		},
		{
			"draft allowed",
			[]Row{{Position: 0, Subject: draft}},
			nil,
			true,
			[]string{draft + "\n"},
			nil,
		},
		{
			"override can clear a draft",
			[]Row{{Position: 0, Subject: draft}},
			map[string]string{"0": "feat: y"},
			false,
			[]string{"feat: y\n"},
			nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Messages(testCase.rows, testCase.subjects, testCase.allowDraft)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Messages() err = %v, want %v", err, testCase.wantErr)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("Messages() = %q, want %q", got, testCase.want)
			}
		})
	}
}
