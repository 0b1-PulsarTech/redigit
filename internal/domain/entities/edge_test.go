package entities

import "testing"

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
