package passes

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestTidyForeignSettlesWithinOneSlot(t *testing.T) {
	t.Parallel()

	home := declSpec{file: "pkg/h/h.go", name: "H", lines: 60}
	testA := declSpec{file: "pkg/a/a_test.go", name: "TestA", lines: 250, test: true}
	testB := declSpec{file: "pkg/b/b_test.go", name: "TestB", lines: 250, test: true}

	const atom = "query:GetUser"

	models := declSpec{file: "internal/db/models.go", name: "Q", kind: entities.KindType, lines: 20}
	app := declSpec{file: "internal/app/app.go", name: "App", lines: 120}
	query := declSpec{
		file: "db/queries/user.sql", name: "GetUser", kind: entities.KindOther,
		lines: 5, atom: atom,
	}
	gen := declSpec{
		file: "internal/db/user.sql.go", name: "GenGetUser", lines: 30, generated: true, atom: atom,
	}
	testUser := declSpec{
		file: "internal/db/user_test.go", name: "TestGetUser", lines: 250, test: true,
	}

	testCases := []tidyCase{
		{
			name: "a foreign block never carries one member of an atom away from its mates",
			rows: [][]declSpec{{models}, {app, query, gen, testUser}},
			edges: []edgeSpec{
				{"GenGetUser", "Q"}, {"TestGetUser", "GenGetUser"}, {"App", "Q"},
			},
			links:        []linkSpec{{edgeSpec{"GenGetUser", "GetUser"}, entities.ReasonQuery}},
			wantSubjects: []string{"row a", "row b"},
		},
		{
			name:  "a block held back by a sibling block moves in the same run",
			rows:  [][]declSpec{{home, testA, testB}},
			edges: []edgeSpec{{"TestA", "H"}, {"TestB", "TestA"}},
			wantSubjects: []string{
				"row a", "DRAFT: test(a): a_test.go", "DRAFT: test(b): b_test.go",
			},
			wantMoves: 2,
		},
		{
			name: "a commit of only web files and tests has no home and makes no move",
			rows: [][]declSpec{
				{{file: "pkg/h/h.go", name: "H", lines: 20}},
				{{file: "pkg/a/view.ts", name: "view", kind: entities.KindOther, lines: 30}, testA},
			},
			edges:        []edgeSpec{{"TestA", "H"}},
			wantSubjects: []string{"row a", "row b"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, moves := applyTwice(t, testCase)
			if !slices.Equal(got, testCase.wantSubjects) || moves != testCase.wantMoves {
				t.Fatalf(
					"subjects = %v, %d moves, want %v, %d",
					got, moves, testCase.wantSubjects, testCase.wantMoves,
				)
			}
		})
	}
}
