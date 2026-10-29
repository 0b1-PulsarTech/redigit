package rules

import (
	"encoding/json"
	"testing"
)

func TestDecodeStrict(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{"an array of strings", `["a"]`, ""},
		{"null is no value", `null`, "expected an array of strings"},
		{"null with blanks is no value", " null ", "expected an array of strings"},
		{"the wrong kind", `{"a":1}`, "expected an array of strings"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var dst []string

			err := DecodeStrict(json.RawMessage(testCase.raw), &dst, "an array of strings")
			if got := errText(err); got != testCase.wantErr {
				t.Fatalf("DecodeStrict() err = %q, want %q", got, testCase.wantErr)
			}
		})
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}
