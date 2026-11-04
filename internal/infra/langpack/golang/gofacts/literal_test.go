package gofacts

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

func TestIndexCallsRecordTheCallAFuncLiteralWasPassedTo(t *testing.T) {
	t.Parallel()

	fix := indexOf(t, map[string]string{
		"a.go": `package fixture

func Reg(fns ...any) {}
func Hold(fns ...any) {}
func Plain() {}
func Other() int { return 0 }
func single() { Reg(func() { Plain() }) }
func nested() { Reg(func() { Hold(func() { Plain() }); Plain() }) }
func gap() { Reg(func() {}, Other(), func() { Plain() }) }
func argBeforeLiteral() { Reg(func() { Hold(Other(), func() { Plain() }) }) }
func afterLiteral() { Reg(func() { Plain() }); Plain() }
func twoLiterals() { Reg(func() { Plain() }, func() { Plain() }) }
func bare() { Reg(Plain); Plain() }
func deep() { Reg(func() { Hold(func() { Hold(func() { Plain() }) }) }) }
`,
	})

	testCases := []struct {
		name string
		site string
		want []string
	}{
		{
			name: "a call in a literal names the call",
			site: "single",
			want: []string{"Reg", "Plain<-Reg"},
		},
		{
			name: "a literal in a literal resolves to its nearest call",
			site: "nested",
			want: []string{"Reg", "Hold<-Reg", "Plain<-Hold", "Plain<-Reg"},
		},
		{
			name: "a call between two literals of one call is not within it",
			site: "gap",
			want: []string{"Reg", "Other", "Plain<-Reg"},
		},
		{
			name: "an argument beside a nested literal is within the outer call",
			site: "argBeforeLiteral",
			want: []string{"Reg", "Hold<-Reg", "Other<-Reg", "Plain<-Hold"},
		},
		{
			name: "a call after the literal ended is not within it",
			site: "afterLiteral",
			want: []string{"Reg", "Plain<-Reg", "Plain"},
		},
		{
			name: "every literal of one call is within it",
			site: "twoLiterals",
			want: []string{"Reg", "Plain<-Reg", "Plain<-Reg"},
		},
		{name: "a func value is not a literal", site: "bare", want: []string{"Reg", "Plain"}},
		{
			name: "a chain of literals keeps one link",
			site: "deep",
			want: []string{"Reg", "Hold<-Reg", "Hold<-Hold", "Plain<-Hold"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := callLines(t, fix.index.Calls(funcKey("a.go", testCase.site)))

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("Calls(%s) = %v, want %v", testCase.site, got, testCase.want)
			}
		})
	}
}

func callLines(t *testing.T, calls []facts.Call) []string {
	t.Helper()

	got := make([]string, 0, len(calls))

	for _, call := range calls {
		line := strings.TrimPrefix(call.Callee, module+".")
		if call.Within != nil {
			line += "<-" + strings.TrimPrefix(call.Within.Callee, module+".")

			if call.Within.Within != nil {
				t.Fatalf("Within of %s keeps a chain: %+v", line, call.Within)
			}
		}

		got = append(got, line)
	}

	return got
}
