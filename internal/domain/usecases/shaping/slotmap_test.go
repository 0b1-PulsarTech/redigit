package shaping

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func key(name string) entities.DeclKey {
	return entities.DeclKey{File: name + ".go", Kind: entities.KindFunc, Name: name}
}

func TestSlotMap(t *testing.T) {
	t.Parallel()

	type placement struct {
		name string
		slot int
	}

	testCases := []struct {
		name       string
		placed     []placement
		merges     [][2]int
		ask        string
		wantSlot   int
		wantFound  bool
		wantResolv map[int]int
	}{
		{"none placed", nil, nil, "a", 0, false, nil},
		{"one placed", []placement{{"a", 3}}, nil, "a", 3, true, map[int]int{3: 3}},
		{
			"merge redirects the merged slot",
			[]placement{{"a", 1}, {"b", 2}},
			[][2]int{{1, 2}},
			"a", 2, true,
			map[int]int{1: 2, 2: 2},
		},
		{
			"alias chain resolves to the last",
			[]placement{{"a", 1}},
			[][2]int{{1, 2}, {2, 5}},
			"a", 5, true,
			map[int]int{1: 5, 2: 5, 5: 5},
		},
		{
			"merge into an alias of itself does not loop",
			[]placement{{"a", 1}},
			[][2]int{{1, 2}, {2, 1}},
			"a", 2, true,
			map[int]int{1: 2, 2: 2},
		},
		{
			"replace moves a key",
			[]placement{{"a", 1}, {"a", 7}},
			nil, "a", 7, true,
			map[int]int{7: 7},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			slots := NewSlotMap()
			for _, put := range testCase.placed {
				slots.Place(key(put.name), put.slot)
			}

			for _, merge := range testCase.merges {
				slots.Merge(merge[0], merge[1])
			}

			got, found := slots.Slot(key(testCase.ask))
			if got != testCase.wantSlot || found != testCase.wantFound {
				t.Fatalf(
					"Slot() = %d, %v, want %d, %v",
					got,
					found,
					testCase.wantSlot,
					testCase.wantFound,
				)
			}

			for from, want := range testCase.wantResolv {
				if resolved := slots.Resolve(from); resolved != want {
					t.Errorf("Resolve(%d) = %d, want %d", from, resolved, want)
				}
			}
		})
	}
}

func TestSlotMapClone(t *testing.T) {
	t.Parallel()

	slots := NewSlotMap()
	slots.Place(key("a"), 1)

	clone := slots.Clone()
	clone.Place(key("zz"), 99)
	clone.Merge(1, 2)

	if _, leaked := slots.Slot(key("zz")); leaked {
		t.Fatal("Clone() shares its placements with the original")
	}

	if got, _ := slots.Slot(key("a")); got != 1 {
		t.Fatalf("Slot(a) = %d after the clone merged, want 1", got)
	}
}
