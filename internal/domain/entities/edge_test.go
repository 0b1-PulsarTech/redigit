package entities

import "testing"

func TestIsLinkOnly(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		reason EdgeReason
		want   bool
	}{
		{ReasonImport, true},
		{ReasonProvides, true},
		{ReasonRegistered, true},
		{ReasonModule, true},
		{ReasonChecksum, true},
		{ReasonReference, false},
		{ReasonBody, false},
		{EdgeReason("unknown"), false},
	}

	for _, testCase := range testCases {
		t.Run(string(testCase.reason), func(t *testing.T) {
			t.Parallel()

			if got := IsLinkOnly(testCase.reason); got != testCase.want {
				t.Fatalf("IsLinkOnly(%q) = %v, want %v", testCase.reason, got, testCase.want)
			}
		})
	}
}

func TestIsHard(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		reason EdgeReason
		want   bool
	}{
		{ReasonReference, true},
		{ReasonImplements, false},
		{ReasonMethodSet, true},
		{ReasonGeneratedBy, false},
		{ReasonMirrors, true},
		{ReasonTestOf, false},
		{ReasonEncloses, true},
		{ReasonSchema, true},
		{ReasonQuery, true},
		{ReasonHosts, true},
		{ReasonDescribes, false},
		{ReasonModule, true},
		{ReasonWorkspace, true},
		{ReasonImport, true},
		{ReasonChecksum, true},
		{ReasonEmbed, true},
		{ReasonProse, false},
		{ReasonBody, true},
		{ReasonProvides, true},
		{ReasonRegistered, true},
		{ReasonPin, true},
		{EdgeReason("unknown"), false},
	}

	for _, testCase := range testCases {
		t.Run(string(testCase.reason), func(t *testing.T) {
			t.Parallel()

			if got := IsHard(testCase.reason); got != testCase.want {
				t.Fatalf("IsHard(%q) = %v, want %v", testCase.reason, got, testCase.want)
			}
		})
	}
}

func TestDeclKindIsCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		kind DeclKind
		want bool
	}{
		{name: "a function is code", kind: KindFunc, want: true},
		{name: "a method is code", kind: KindMethod, want: true},
		{name: "a type is code", kind: KindType, want: true},
		{name: "a value is code", kind: KindValue, want: true},
		{name: "a statement is code", kind: KindStmt, want: true},
		{name: "other is not code", kind: KindOther},
		{name: "an unknown kind is not code", kind: DeclKind("manifest")},
		{name: "no kind is not code"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.kind.IsCode(); got != testCase.want {
				t.Fatalf("%q.IsCode() = %t, want %t", testCase.kind, got, testCase.want)
			}
		})
	}
}
