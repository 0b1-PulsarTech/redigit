package passes

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func jointCases() []tidyCase {
	code := declSpec{file: "pkg/a/a.go", name: "A", lines: 90}
	testA := declSpec{file: "pkg/a/a_test.go", name: "TestA", lines: 90, test: true}
	fake := declSpec{
		file: "pkg/b/fake_test.go", name: "fakeStore", kind: entities.KindType,
		lines: 250, test: true,
	}
	host := declSpec{file: "pkg/a/c.go", name: "C", lines: 70}
	port := declSpec{
		file: "pkg/b/port_test.go", name: "store", kind: entities.KindType,
		lines: 40, test: true, iface: true,
	}
	tail := declSpec{file: "pkg/c/c.go", name: "Z", lines: 30}

	return []tidyCase{{
		name:  "a soft provider pulled forward by the tail is settled in one application",
		rows:  [][]declSpec{{code}, {testA}, {fake, host}, {port}, {tail}},
		edges: []edgeSpec{{"TestA", "A"}, {"store", "TestA"}},
		links: []linkSpec{{edgeSpec{"fakeStore", "store"}, entities.ReasonImplements}},
		runs:  2,
		wantSubjects: []string{
			"row a", "row b", "row d", "DRAFT: test(b): fake_test.go", "row c", "row e",
		},
	}}
}

func orderCases() []tidyCase {
	prod := declSpec{file: "pkg/a/a.go", name: "P", lines: 50}
	other := declSpec{file: "pkg/n/n.go", name: "N", lines: 50}
	testA := declSpec{file: "pkg/a/x_test.go", name: "TestXA", lines: 50, test: true}
	testB := declSpec{file: "pkg/b/x_test.go", name: "TestXB", lines: 50, test: true}
	later := declSpec{file: "pkg/a/y_test.go", name: "TestY", lines: 50, test: true}

	return []tidyCase{{
		name:  "pieces spawned out of order still keep the plan order of the trailing tests",
		rows:  [][]declSpec{{prod}, {other}, {testA, testB}, {later}},
		edges: []edgeSpec{{"TestXA", "P"}, {"TestXB", "P"}, {"TestY", "P"}},
		wantSubjects: []string{
			"row a", "DRAFT: test(a): x_test.go", "DRAFT: test(b): x_test.go", "row d", "row b",
		},
		wantMoves: 4,
	}}
}

func reviewCases() []tidyCase {
	return slices.Concat(jointCases(), orderCases())
}
