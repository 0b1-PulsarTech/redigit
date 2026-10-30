package entities

import "testing"

func sameStorage[Item any](left, right []Item) bool {
	return len(left) > 0 && len(right) > 0 && &left[0] == &right[0]
}

func TestGraphSliceAccessorsShareTheirStorage(t *testing.T) {
	t.Parallel()

	first := Decl{Key: DeclKey{File: "a.go", Kind: KindFunc, Name: "A"}, Unit: "a"}
	second := Decl{Key: DeclKey{File: "b.go", Kind: KindFunc, Name: "B"}, Unit: "b"}
	graph := NewGraph(
		[]Decl{first, second},
		[]Edge{{Dependent: first.Key, Provider: second.Key, Reason: ReasonReference}},
		[]Anchor{{Decl: first.Key, To: second.Key, Reason: ReasonTestOf}},
	)

	testCases := []struct {
		name   string
		isSame func() bool
	}{
		{
			"two Edges calls return the same storage",
			func() bool { return sameStorage(graph.Edges(), graph.Edges()) },
		},
		{
			"Needs is a window onto the storage Edges returns",
			func() bool { return sameStorage(graph.Needs(first.Key), graph.Edges()) },
		},
		{
			"two Anchors calls return the same storage",
			func() bool { return sameStorage(graph.Anchors(), graph.Anchors()) },
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !testCase.isSame() {
				t.Fatal("the accessor copied: a clone costs allocations in every planner round")
			}
		})
	}
}
