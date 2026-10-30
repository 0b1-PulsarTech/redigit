package slots

import (
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func TestParseCommits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		out     string
		want    []verifying.Commit
		wantErr error
	}{
		{"empty output", "", []verifying.Commit{}, nil},
		{
			"two commits get positions in order",
			"o1\x00t1\x00first\no2\x00t2\x00[wip] second: x\x00y\n",
			[]verifying.Commit{
				{Oid: "o1", Tree: "t1", Subject: "first", Position: 1},
				{Oid: "o2", Tree: "t2", Subject: "[wip] second: x\x00y", Position: 2},
			},
			nil,
		},
		{"a line with too few fields", "o1\x00t1\n", nil, ErrBadLog},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseCommits([]byte(testCase.out))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if len(got) != len(testCase.want) {
				t.Fatalf("got %+v, want %+v", got, testCase.want)
			}

			for idx := range got {
				if got[idx] != testCase.want[idx] {
					t.Fatalf("commit %d = %+v, want %+v", idx, got[idx], testCase.want[idx])
				}
			}
		})
	}
}
