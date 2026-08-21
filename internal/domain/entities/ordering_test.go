package entities

import "testing"

func TestPositionIsSet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		at   Position
		want bool
	}{
		{"first commit is a real position", 0, true},
		{"later", 500, true},
		{"absent", NoPosition, false},
		{"anything negative is absent", -7, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.at.IsSet(); got != testCase.want {
				t.Fatalf("IsSet() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestOrderingOf(t *testing.T) {
	t.Parallel()

	placed := DeclKey{File: "a.go", Kind: KindFunc, Name: "New"}
	missing := DeclKey{File: "b.go", Kind: KindFunc, Name: "Other"}
	ordering := Ordering{At: map[DeclKey]Position{placed: 12}}

	testCases := []struct {
		name string
		key  DeclKey
		want Position
	}{
		{"placed", placed, 12},
		{"absent reads as NoPosition, never as commit zero", missing, NoPosition},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := ordering.Of(testCase.key); got != testCase.want {
				t.Fatalf("Of() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestOrderingLast(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		ordering Ordering
		want     Position
	}{
		{"empty", Ordering{At: map[DeclKey]Position{}}, NoPosition},
		{
			"highest wins",
			Ordering{At: map[DeclKey]Position{
				{File: "a.go", Name: "A"}: 3,
				{File: "b.go", Name: "B"}: 41,
				{File: "c.go", Name: "C"}: 7,
			}},
			41,
		},
		{
			"all at commit zero",
			Ordering{At: map[DeclKey]Position{{File: "a.go", Name: "A"}: 0}},
			0,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.ordering.Last(); got != testCase.want {
				t.Fatalf("Last() = %d, want %d", got, testCase.want)
			}
		})
	}
}
