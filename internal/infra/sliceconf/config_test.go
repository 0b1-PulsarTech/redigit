package sliceconf

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		raw     string
		want    Config
		wantErr error
	}{
		{name: "an absent section is the zero config", raw: ""},
		{name: "an empty object is the zero config", raw: `{}`},
		{
			name: "both keys are read",
			raw:  `{"functions":["register*","Server.Mount*"],"errcheck":true}`,
			want: Config{Functions: []string{"register*", "Server.Mount*"}, Errcheck: true},
		},
		{
			name:    "an unknown key is refused",
			raw:     `{"functions":["run"],"shellWithFirst":true}`,
			wantErr: ErrInvalid,
		},
		{name: "a section that is not an object is refused", raw: `[]`, wantErr: ErrInvalid},
		{name: "null is refused", raw: `null`, wantErr: ErrInvalid},
		{name: "null padded with whitespace is refused", raw: " null\n", wantErr: ErrInvalid},
		{
			name:    "a function list of the wrong type is refused",
			raw:     `{"functions":"run"}`,
			wantErr: ErrInvalid,
		},
		{
			name:    "errcheck of the wrong type is refused",
			raw:     `{"errcheck":"yes"}`,
			wantErr: ErrInvalid,
		},
		{name: "an empty glob is refused", raw: `{"functions":[""]}`, wantErr: ErrInvalid},
		{name: "a null glob is refused", raw: `{"functions":[null]}`, wantErr: ErrInvalid},
		{
			name:    "a malformed glob is refused",
			raw:     `{"functions":["reg[ister"]}`,
			wantErr: ErrInvalid,
		},
		{name: "trailing data is refused", raw: `{} {}`, wantErr: ErrInvalid},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Decode(json.RawMessage(testCase.raw))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Decode() error = %v, want %v", err, testCase.wantErr)
			}

			if !slices.Equal(got.Functions, testCase.want.Functions) ||
				got.Errcheck != testCase.want.Errcheck {
				t.Fatalf("Decode() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
