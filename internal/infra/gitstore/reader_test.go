package gitstore

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestParseHistory(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		out  []byte
		want []entities.SourceTouch
	}{
		{"no commits at all", nil, nil},
		{
			"one commit, one path",
			[]byte("\x01aaa111\na.go\x00"),
			[]entities.SourceTouch{{Position: 0, Touched: []string{"a.go"}}},
		},
		{
			"three commits in reverse-chronological git order",
			[]byte(
				"\x01sha1\na.go\x00sub dir/b.go\x00\x01sha2\na.go\x00c.go\x00\x01sha3\nsub dir/b.go\x00",
			),
			[]entities.SourceTouch{
				{Position: 0, Touched: []string{"a.go", "sub dir/b.go"}},
				{Position: 1, Touched: []string{"a.go", "c.go"}},
				{Position: 2, Touched: []string{"sub dir/b.go"}},
			},
		},
		{
			"a commit with no touched paths still occupies a position",
			[]byte("\x01sha1\n"),
			[]entities.SourceTouch{{Position: 0, Touched: nil}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := parseHistory(testCase.out)
			if len(got) != len(testCase.want) {
				t.Fatalf("parseHistory() = %+v, want %+v", got, testCase.want)
			}

			for i, touch := range got {
				want := testCase.want[i]
				if touch.Position != want.Position {
					t.Fatalf(
						"parseHistory()[%d].Position = %d, want %d",
						i,
						touch.Position,
						want.Position,
					)
				}

				if len(touch.Touched) != len(want.Touched) {
					t.Fatalf(
						"parseHistory()[%d].Touched = %v, want %v",
						i,
						touch.Touched,
						want.Touched,
					)
				}

				for j, path := range touch.Touched {
					if path != want.Touched[j] {
						t.Fatalf(
							"parseHistory()[%d].Touched[%d] = %q, want %q",
							i,
							j,
							path,
							want.Touched[j],
						)
					}
				}
			}
		})
	}
}
