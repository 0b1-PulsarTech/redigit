package entities

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
)

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

func tiedDecls(mutate func(*Decl)) (low, high Decl) {
	low = Decl{Key: DeclKey{File: "m.go", Kind: KindFunc, Name: "init"}, Unit: "u"}
	high = low
	mutate(&high)

	return low, high
}

func TestNewGraphDuplicateKeys(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(*Decl)
	}{
		{"unit", func(decl *Decl) { decl.Unit = "v" }},
		{"span start", func(decl *Decl) { decl.Span.Start = 4 }},
		{"span end", func(decl *Decl) { decl.Span.End = 4 }},
		{"selector file", func(decl *Decl) { decl.Selector.File = "z.go" }},
		{"selector line", func(decl *Decl) { decl.Selector.Line = 4 }},
		{"selector char", func(decl *Decl) { decl.Selector.Char = 4 }},
		{"atom", func(decl *Decl) { decl.Atom = "z" }},
		{"group size", func(decl *Decl) { decl.GroupSize = 4 }},
		{"lines", func(decl *Decl) { decl.Lines = 4 }},
		{"frame", func(decl *Decl) { decl.Frame = 4 }},
		{"is test", func(decl *Decl) { decl.IsTest = true }},
		{"is interface", func(decl *Decl) { decl.IsInterface = true }},
		{"generated", func(decl *Decl) { decl.Generated = true }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			low, high := tiedDecls(testCase.mutate)
			fillers := make([]Decl, 0, 14)

			for index := range 14 {
				fillers = append(
					fillers,
					Decl{Key: DeclKey{File: "m.go", Kind: KindFunc, Name: fmt.Sprint("f", index)}},
				)
			}

			for _, pair := range [][2]Decl{{low, high}, {high, low}} {
				decls := slices.Concat([]Decl{pair[0]}, fillers[:7], []Decl{pair[1]}, fillers[7:])

				graph := NewGraph(decls, nil, nil)
				if got, _ := graph.Decl(low.Key); got != low {
					t.Fatalf("Decl() = %+v, want %+v for any order", got, low)
				}
			}
		})
	}
}

func setLeaf(
	t *testing.T, value reflect.Value, prefix string, visited *int, target int,
) (path string) {
	t.Helper()

	for info, field := range value.Fields() {
		name := prefix + info.Name

		if field.Kind() == reflect.Struct {
			if found := setLeaf(t, field, name+".", visited, target); found != "" {
				path = found
			}

			continue
		}

		if *visited == target {
			if !field.CanSet() {
				t.Fatalf("field %s cannot be set", name)
			}

			switch field.Kind() {
			case reflect.String:
				field.SetString("z")
			case reflect.Bool:
				field.SetBool(true)
			case reflect.Int:
				field.SetInt(7)
			default:
				t.Fatalf("field %s has unsupported kind %s", name, field.Kind())
			}

			path = name
		}

		*visited++
	}

	return path
}

func TestCompareDeclsSeesEveryField(t *testing.T) {
	t.Parallel()

	leafCount := 0
	setLeaf(t, reflect.ValueOf(&Decl{}).Elem(), "", &leafCount, -1)

	if leafCount == 0 {
		t.Fatal("found no fields in Decl")
	}

	testCases := make([]struct {
		name string
		leaf int
	}, leafCount)
	for leaf := range testCases {
		visited := 0
		testCases[leaf].name = setLeaf(t, reflect.ValueOf(&Decl{}).Elem(), "", &visited, leaf)
		testCases[leaf].leaf = leaf
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var changed Decl

			visited := 0
			setLeaf(t, reflect.ValueOf(&changed).Elem(), "", &visited, testCase.leaf)

			if compareDecls(Decl{}, changed) == 0 || compareDecls(changed, Decl{}) == 0 {
				t.Fatalf("compareDecls() cannot tell apart %+v", changed)
			}
		})
	}
}
