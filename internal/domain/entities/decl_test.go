package entities

import "testing"

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
