package pinrule

import (
	"errors"
	"testing"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		raw     string
		wantErr error
	}{
		{name: "a pin", raw: `{"type":"pin","decl":"a.go:func:A","after":"b.go:func:B"}`},
		{
			name:    "an unknown key",
			raw:     `{"type":"pin","decl":"a.go:func:A","after":"b.go:func:B","bogus":1}`,
			wantErr: ErrConfig,
		},
		{
			name:    "with is not implemented and so refused",
			raw:     `{"type":"pin","decl":"a.go:func:A","after":"b.go:func:B","with":"c.go:func:C"}`,
			wantErr: ErrConfig,
		},
		{
			name:    "a stmt decl",
			raw:     `{"type":"pin","decl":"main.go:stmt:main#1","after":"b.go:func:B"}`,
			wantErr: ErrStmtPattern,
		},
		{
			name:    "a stmt after",
			raw:     `{"type":"pin","decl":"a.go:func:A","after":"main.go:stmt:main#1"}`,
			wantErr: ErrStmtPattern,
		},
		{
			name: "a kind wild glob still decodes",
			raw:  `{"type":"pin","decl":"a.go:*:A","after":"b.go:*:B"}`,
		},
		{name: "no after", raw: `{"type":"pin","decl":"a.go:func:A"}`, wantErr: ErrMissingField},
		{name: "no decl", raw: `{"type":"pin","after":"a.go:func:A"}`, wantErr: ErrMissingField},
		{name: "a null", raw: `null`, wantErr: ErrMissingField},
		{name: "not an object", raw: `[1]`, wantErr: ErrConfig},
		{
			name:    "a malformed decl",
			raw:     `{"type":"pin","decl":"a.go","after":"b.go:func:B"}`,
			wantErr: ErrBadPattern,
		},
		{
			name:    "a malformed after",
			raw:     `{"type":"pin","decl":"a.go:func:A","after":"b[.go:func:B"}`,
			wantErr: ErrBadPattern,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := decode([]byte(testCase.raw))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("decode() err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
