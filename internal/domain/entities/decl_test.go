package entities

import (
	"cmp"
	"strings"
	"testing"
)

func TestUnitIDDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		unit UnitID
		want string
	}{
		{"plain directory", "modules/lead/usecases", "modules/lead/usecases"},
		{"external test unit keeps only the directory", "modules/lead#lead_test", "modules/lead"},
		{"empty", "", ""},
		{"marker with empty name", "modules/lead#", "modules/lead"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.unit.Dir(); got != testCase.want {
				t.Fatalf("Dir() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestDeclKeyString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		key  DeclKey
		want string
	}{
		{"function", DeclKey{File: "a.go", Kind: KindFunc, Name: "New"}, "a.go:func:New"},
		{
			"method keeps the receiver so two types with one method name stay apart",
			DeclKey{File: "a.go", Kind: KindMethod, Name: "Repo.Insert"},
			"a.go:method:Repo.Insert",
		},
		{"zero", DeclKey{}, "::"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.key.String(); got != testCase.want {
				t.Fatalf("String() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestDeclKeyIsZero(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		key  DeclKey
		want bool
	}{
		{"zero", DeclKey{}, true},
		{"only a file", DeclKey{File: "a.go"}, false},
		{"only a name", DeclKey{Name: "New"}, false},
		{"complete", DeclKey{File: "a.go", Kind: KindType, Name: "Lead"}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.key.IsZero(); got != testCase.want {
				t.Fatalf("IsZero() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestCompareKeys(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		left, right DeclKey
		want        int
	}{
		{"equal", DeclKey{"a.go", KindFunc, "X"}, DeclKey{"a.go", KindFunc, "X"}, 0},
		{"both zero", DeclKey{}, DeclKey{}, 0},
		{"zero before anything", DeclKey{}, DeclKey{File: "a.go"}, -1},
		{"empty name before a name", DeclKey{"a", KindFunc, ""}, DeclKey{"a", KindFunc, "X"}, -1},
		{"empty kind before a kind", DeclKey{"a", "", "X"}, DeclKey{"a", KindFunc, "X"}, -1},
		{"file differs", DeclKey{"a.go", KindFunc, "Z"}, DeclKey{"b.go", KindFunc, "A"}, -1},
		{"kind differs", DeclKey{"a.go", KindType, "X"}, DeclKey{"a.go", KindFunc, "X"}, 1},
		{"name differs", DeclKey{"a.go", KindFunc, "A"}, DeclKey{"a.go", KindFunc, "B"}, -1},
		{
			"Dockerfile prefix of Dockerfile.dev, dot below colon",
			DeclKey{"Dockerfile", KindOther, "x"},
			DeclKey{"Dockerfile.dev", KindOther, "x"},
			1,
		},
		{
			"Dockerfile.dev before Dockerfile, reversed",
			DeclKey{"Dockerfile.dev", KindOther, "x"},
			DeclKey{"Dockerfile", KindOther, "x"},
			-1,
		},
		{
			"LICENSE prefix of LICENSE.md",
			DeclKey{"LICENSE", KindOther, "x"},
			DeclKey{"LICENSE.md", KindOther, "x"},
			1,
		},
		{
			"a prefix of a:b, colon inside the file",
			DeclKey{"a", KindFunc, "x"},
			DeclKey{"a:b", KindFunc, "x"},
			1,
		},
		{
			"kind prefix of another kind",
			DeclKey{"a.go", "fun", "x"},
			DeclKey{"a.go", "func", "x"},
			-1,
		},
		{
			"kind prefix with a name below the colon",
			DeclKey{"a.go", "f", "!"},
			DeclKey{"a.go", "f!", "x"},
			1,
		},
		{
			"same String, file and kind decide",
			DeclKey{"x:func", KindType, "T"},
			DeclKey{"x", KindFunc, "type:T"},
			1,
		},
		{
			"name prefix of another name",
			DeclKey{"a.go", KindFunc, "Run"},
			DeclKey{"a.go", KindFunc, "Runner"},
			-1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := CompareKeys(testCase.left, testCase.right); got != testCase.want {
				t.Fatalf("CompareKeys() = %d, want %d", got, testCase.want)
			}

			byText := strings.Compare(testCase.left.String(), testCase.right.String())
			if byText != 0 && byText != testCase.want {
				t.Fatalf("fixture disagrees with String order: %d, want %d", byText, testCase.want)
			}
		})
	}
}

func FuzzCompareKeys(f *testing.F) {
	f.Add("Dockerfile", "func", "x", "Dockerfile.dev", "func", "x")
	f.Add("a", "", "", "a:b", "", "")
	f.Add("", "f", "!", "", "f!", "x")
	f.Add("x:func", "type", "T", "x", "func", "type:T")

	f.Fuzz(
		func(t *testing.T, leftFile, leftKind, leftName, rightFile, rightKind, rightName string) {
			left := DeclKey{leftFile, DeclKind(leftKind), leftName}
			right := DeclKey{rightFile, DeclKind(rightKind), rightName}

			wantSign := cmp.Compare(strings.Compare(left.String(), right.String()), 0)
			got := CompareKeys(left, right)

			if wantSign != 0 && got != wantSign {
				t.Fatalf("CompareKeys(%q, %q) = %d, want %d", left, right, got, wantSign)
			}

			if (got == 0) != (left == right) {
				t.Fatalf("CompareKeys(%q, %q) = %d, equal: %t", left, right, got, left == right)
			}

			if back := CompareKeys(right, left); back != -got {
				t.Fatalf("CompareKeys(%q, %q) = %d, reversed %d", left, right, got, back)
			}
		},
	)
}

func TestCompareKeysAllocatesNothing(t *testing.T) {
	left := DeclKey{"Dockerfile", KindOther, "x"}
	right := DeclKey{"Dockerfile.dev", KindOther, "x"}

	if got := testing.AllocsPerRun(100, func() { CompareKeys(left, right) }); got != 0 {
		t.Fatalf("CompareKeys() allocates %v times, want 0", got)
	}
}

func BenchmarkCompareKeys(b *testing.B) {
	left := DeclKey{"internal/domain/Dockerfile", KindMethod, "Repo.Insert"}
	right := DeclKey{"internal/domain/Dockerfile.dev", KindMethod, "Repo.Insert"}

	b.ReportAllocs()

	for b.Loop() {
		CompareKeys(left, right)
	}
}
