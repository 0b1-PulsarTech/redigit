package anchoring

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func logged(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var out []map[string]any

	for line := range strings.Lines(buf.String()) {
		fields := map[string]any{}
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}

		out = append(out, fields)
	}

	return out
}

func TestSummaryPerAnchor(t *testing.T) {
	t.Parallel()

	fix := Anchor{Rev: "fix", Position: 5}
	wip := Anchor{Rev: "fix", Position: 5, Policy: PolicyWIP}
	broken := Verdict{Detail: "undefined: Z\nmore"}

	testCases := []struct {
		name        string
		anchor      Anchor
		verdict     Verdict
		wantOutcome string
		wantReason  string
	}{
		{
			name: "a pre-image that builds", anchor: fix, verdict: Verdict{IsBuilt: true},
			wantOutcome: "kept", wantReason: "its pre-image builds",
		},
		{
			name: "a pre-image that fails under fold", anchor: fix, verdict: broken,
			wantOutcome: "folded", wantReason: "undefined: Z",
		},
		{
			name: "a pre-image that fails under wip", anchor: wip, verdict: broken,
			wantOutcome: "wip", wantReason: "undefined: Z",
		},
		{
			name: "a tree that gives no verdict", anchor: fix, wantOutcome: "unchecked",
			verdict: Verdict{IsUnchecked: true, Detail: "stale"}, wantReason: "stale",
		},
		{
			name: "an anchor that changes nothing", anchor: Anchor{Rev: "noop", Position: 5},
			verdict: Verdict{IsBuilt: true}, wantOutcome: "dropped",
			wantReason: "it changes no placed declaration",
		},
		{
			name: "a declaration the final tree lost", anchor: Anchor{Rev: "lost", Position: 5},
			verdict: Verdict{IsBuilt: true}, wantOutcome: "dropped",
			wantReason: "it changes no placed declaration",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				record probeLog
				buf    bytes.Buffer
			)

			ctrl := gomock.NewController(t)
			probe := scripted(ctrl, &record, []Verdict{testCase.verdict}, nil)
			planner := fixture(ctrl, probe, "", testCase.anchor)
			planner.cfg.Logger = slog.New(slog.NewJSONHandler(&buf, nil))

			if _, err := planOf(planner, finalFiles()); err != nil {
				t.Fatalf("Plan() = %v", err)
			}

			var summaries []map[string]any

			for _, fields := range logged(t, &buf) {
				if fields["msg"] == "anchors: summary" {
					summaries = append(summaries, fields)
				}
			}

			if len(summaries) != 1 {
				t.Fatalf("summaries = %v, want exactly one", summaries)
			}

			got := summaries[0]
			isRight := got["level"] == "WARN" && got["rev"] == testCase.anchor.Rev
			isRight = isRight && got["outcome"] == testCase.wantOutcome

			if !isRight || got["reason"] != testCase.wantReason {
				t.Fatalf("summary = %v, want %s", got, testCase.wantOutcome)
			}
		})
	}
}

func TestSkippedDeclarationsAreWarned(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		anchor    Anchor
		files     map[string]string
		wantMsg   string
		wantKey   string
		wantValue any
	}{
		{
			name:      "a declaration the final tree lacks is warned as not spliced",
			anchor:    Anchor{Rev: "fix", Position: 5},
			files:     map[string]string{"a.go": "B other\n"},
			wantMsg:   "anchors: not spliced",
			wantKey:   "reason",
			wantValue: reasonGone,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			ctrl := gomock.NewController(t)
			planner := fixture(ctrl, NewMockProbe(ctrl), "", testCase.anchor)
			planner.cfg.Logger = slog.New(slog.NewJSONHandler(&buf, nil))
			placed := map[entities.DeclKey]bool{fkey("A"): true}

			_, err := planner.collect(context.Background(), graphOf(testCase.files), placed)
			if err != nil {
				t.Fatalf("collect() = %v", err)
			}

			for _, fields := range logged(t, &buf) {
				if fields["msg"] == testCase.wantMsg && fields["level"] == "WARN" &&
					fields[testCase.wantKey] == testCase.wantValue {
					return
				}
			}

			t.Fatalf("no Warn for the skipped declaration in %s", buf.String())
		})
	}
}

func TestFoldNamesEachDeclaration(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		anchor    Anchor
		verdict   Verdict
		wantDecls []string
	}{
		{
			name:      "a pre-image that fails under fold",
			anchor:    Anchor{Rev: "fix", Position: 5},
			verdict:   Verdict{Detail: "undefined: Z"},
			wantDecls: []string{fkey("A").String()},
		},
		{
			name:      "a pre-image that cannot be rendered folds under wip too",
			anchor:    Anchor{Rev: "fix", Position: 5, Policy: PolicyWIP},
			verdict:   Verdict{IsUnfit: true, Detail: "render"},
			wantDecls: []string{fkey("A").String()},
		},
		{
			name:    "a pre-image that builds names nothing",
			anchor:  Anchor{Rev: "fix", Position: 5},
			verdict: Verdict{IsBuilt: true},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				record probeLog
				buf    bytes.Buffer
				named  []string
			)

			ctrl := gomock.NewController(t)
			planner := fixture(
				ctrl,
				scripted(ctrl, &record, []Verdict{testCase.verdict}, nil),
				"",
				testCase.anchor,
			)
			planner.cfg.Logger = slog.New(slog.NewJSONHandler(&buf, nil))

			if _, err := planOf(planner, finalFiles()); err != nil {
				t.Fatalf("Plan() = %v", err)
			}

			for _, fields := range logged(t, &buf) {
				if fields["msg"] == "anchors: folded declaration" && fields["level"] == "INFO" {
					named = append(named, fields["decl"].(string))
				}
			}

			if !slices.Equal(named, testCase.wantDecls) {
				t.Fatalf("folded declarations = %v, want %v", named, testCase.wantDecls)
			}
		})
	}
}

func TestSkipNamesTheFileWhenNoDeclarationIsKnown(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		skipped  skip
		wantKey  string
		wantFile string
	}{
		{
			name:    "a declaration is named by its key",
			skipped: skip{key: fkey("A"), reason: reasonGone}, wantKey: fkey("A").String(),
		},
		{
			name:     "an unparsed file is named by its path, not an empty key",
			skipped:  skip{key: entities.DeclKey{File: "c.go"}, reason: reasonUnparsed},
			wantFile: "c.go",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			planner := New(Config{Logger: slog.New(slog.NewJSONHandler(&buf, nil))})
			planner.reportSkip("fix", testCase.skipped)

			got := logged(t, &buf)
			if len(got) != 1 {
				t.Fatalf("log = %v, want one line", got)
			}

			gotKey, _ := got[0]["decl"].(string)
			gotFile, _ := got[0]["file"].(string)

			if gotKey != testCase.wantKey || gotFile != testCase.wantFile {
				t.Fatalf("log = %v, want decl %q file %q",
					got, testCase.wantKey, testCase.wantFile)
			}
		})
	}
}

func TestBornDeclarationAbsentFromTheFinalTreeIsWarned(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		anchor   Anchor
		files    map[string]string
		wantMsg  string
		wantDecl string
	}{
		{
			name:     "a born declaration the final tree lacks is warned as lost",
			anchor:   Anchor{Rev: "birth", Position: 5},
			files:    map[string]string{"a.go": "A last\n"},
			wantMsg:  "anchors: born declaration not in the final tree",
			wantDecl: fkey("H").String(),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			ctrl := gomock.NewController(t)
			planner := fixture(ctrl, NewMockProbe(ctrl), "", testCase.anchor)
			planner.cfg.Logger = slog.New(slog.NewJSONHandler(&buf, nil))
			placed := map[entities.DeclKey]bool{fkey("A"): true}

			_, err := planner.collect(context.Background(), graphOf(testCase.files), placed)
			if err != nil {
				t.Fatalf("collect() = %v", err)
			}

			for _, fields := range logged(t, &buf) {
				if fields["msg"] == testCase.wantMsg &&
					fields["level"] == "WARN" && fields["decl"] == testCase.wantDecl {
					return
				}
			}

			t.Fatalf("no Warn for the lost born declaration in %s", buf.String())
		})
	}
}

func TestWIPSummaryCountsTheMarkedCommits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		anchor     Anchor
		verdicts   []Verdict
		wantMarked float64
	}{
		{
			name:       "a wip anchor marks the one commit before it",
			anchor:     Anchor{Rev: "fix", Position: 5, Policy: PolicyWIP},
			verdicts:   []Verdict{{Detail: "broken"}},
			wantMarked: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				record probeLog
				buf    bytes.Buffer
			)

			ctrl := gomock.NewController(t)
			probe := scripted(ctrl, &record, testCase.verdicts, nil)
			planner := fixture(ctrl, probe, "", testCase.anchor)
			planner.cfg.Logger = slog.New(slog.NewJSONHandler(&buf, nil))

			if _, err := planOf(planner, finalFiles()); err != nil {
				t.Fatalf("Plan() = %v", err)
			}

			for _, fields := range logged(t, &buf) {
				if fields["msg"] == "anchors: summary" && fields["marked"] == testCase.wantMarked {
					return
				}
			}

			t.Fatalf("no summary marking %v commit in %s", testCase.wantMarked, buf.String())
		})
	}
}

func TestSummaryDropsAnAnchorThatCancelsAgainstTheNext(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		anchors      []Anchor
		verdicts     []Verdict
		wantOutcomes map[string]string
	}{
		{
			name:         "an anchor undone by the next is dropped and the next is kept",
			anchors:      []Anchor{{Rev: "fix", Position: 5}, {Rev: "twin", Position: 7}},
			verdicts:     []Verdict{{IsBuilt: true}},
			wantOutcomes: map[string]string{"fix": "dropped", "twin": "kept"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				record probeLog
				buf    bytes.Buffer
			)

			ctrl := gomock.NewController(t)
			probe := scripted(ctrl, &record, testCase.verdicts, nil)
			planner := fixture(ctrl, probe, "", testCase.anchors...)
			planner.cfg.Logger = slog.New(slog.NewJSONHandler(&buf, nil))

			if _, err := planOf(planner, finalFiles()); err != nil {
				t.Fatalf("Plan() = %v", err)
			}

			outcomes := map[any]any{}

			for _, fields := range logged(t, &buf) {
				if fields["msg"] == "anchors: summary" {
					outcomes[fields["rev"]] = fields["outcome"]
				}
			}

			for rev, want := range testCase.wantOutcomes {
				if outcomes[rev] != want {
					t.Fatalf("outcomes = %v, want %v", outcomes, testCase.wantOutcomes)
				}
			}
		})
	}
}

func TestVersionOfCarriesTheImports(t *testing.T) {
	t.Parallel()

	placed := entry{preimage: "A one", imports: []string{"bytes", `str "strings"`}}
	want := entities.Version{Text: "A one", Until: 3, Imports: []string{"bytes", `str "strings"`}}

	if got := versionOf(placed, 3); !reflect.DeepEqual(got, want) {
		t.Fatalf("versionOf() = %+v, want %+v", got, want)
	}
}

func TestTouches(t *testing.T) {
	t.Parallel()

	home := entities.Decl{Key: fkey("H"), Unit: "h"}
	alias := []string{`z "example.com/p/lib"`}

	testCases := []struct {
		name     string
		preimage string
		unit     entities.UnitID
		imports  []string
		want     bool
	}{
		{name: "a method of any receiver", preimage: "b.Run()", unit: "h", want: true},
		{name: "a method named but not selected", preimage: "Run(b)", unit: "h"},
		{name: "an imported method", preimage: "b.Run()", unit: "lib", imports: alias, want: true},
		{name: "an unrelated method", preimage: "b.Run()", unit: "lib", imports: []string{`"os"`}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			key := entities.DeclKey{File: "x.go", Kind: entities.KindMethod, Name: "Box.Run"}
			placed := entry{key: home.Key, preimage: testCase.preimage, imports: testCase.imports}
			decl := entities.Decl{Key: key, Unit: testCase.unit}

			if got := (scene{}).touches(placed, home, decl); got != testCase.want {
				t.Fatalf("touches() = %t, want %t", got, testCase.want)
			}
		})
	}
}
