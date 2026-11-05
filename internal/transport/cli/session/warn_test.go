package session

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

func TestWarnShortfalls(t *testing.T) {
	blind := []indexing.Blind{{Decl: declKey(), Query: "references", Reason: "timed out"}}
	partial := []indexing.Partial{
		{Broken: 3, Of: 40, Tags: []string{"integration"}},
		{Broken: 1, Of: 12},
	}

	testCases := []struct {
		name   string
		result indexing.Result
		want   []string
	}{
		{"nothing is short", indexing.Result{Graph: declGraph()}, nil},
		{
			"blind spots",
			indexing.Result{Graph: declGraph(), Blind: blind},
			[]string{"the resolver could not answer for some declarations"},
		},
		{
			"partial passes, one line each, tags as the pass had them",
			indexing.Result{Graph: declGraph(), Partial: partial},
			[]string{
				`msg="some packages did not type-check; their type facts are partial" count=3 of=40 tags=[integration]`,
				`msg="some packages did not type-check; their type facts are partial" count=1 of=12 tags=[]`,
			},
		},
		{
			"both, partial first",
			indexing.Result{Graph: declGraph(), Blind: blind, Partial: partial[:1]},
			[]string{
				`msg="some packages did not type-check; their type facts are partial" count=3 of=40 tags=[integration]`,
				"the resolver could not answer for some declarations",
			},
		},
	}

	for _, testCase := range testCases {
		var logged bytes.Buffer

		restore := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{
			ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
				if attr.Key == slog.TimeKey || attr.Key == slog.LevelKey {
					return slog.Attr{}
				}

				return attr
			},
		})))

		WarnShortfalls(testCase.result)
		slog.SetDefault(restore)

		lines := strings.Split(strings.TrimSpace(logged.String()), "\n")

		if len(testCase.want) == 0 && logged.Len() == 0 {
			continue
		}

		if len(lines) != len(testCase.want) {
			t.Fatalf("%s: logged %q, want %d lines", testCase.name, lines, len(testCase.want))
		}

		for idx, want := range testCase.want {
			if !strings.Contains(lines[idx], strings.TrimSpace(want)) {
				t.Errorf(
					"%s: line %d = %q, want it to hold %q",
					testCase.name,
					idx,
					lines[idx],
					want,
				)
			}
		}
	}
}

func TestWarnClosed(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{"a clean close", nil, ""},
		{
			"a checkout kept because the session did not stop",
			errors.New("keep checkout /tmp/redigit-co123: close language server: killed"),
			`msg="the pipeline did not close cleanly; a throwaway checkout may remain" ` +
				`error="keep checkout /tmp/redigit-co123: close language server: killed"`,
		},
	}

	for _, testCase := range testCases {
		logged := captureWarnings(t)

		WarnClosed(testCase.err)

		if got := strings.TrimSpace(logged.String()); got != testCase.want {
			t.Fatalf("%s: logged %q, want %q", testCase.name, got, testCase.want)
		}
	}
}

func TestWarnShortfallsNamesTheFirstBlindSpot(t *testing.T) {
	logged := captureWarnings(t)
	other := entities.DeclKey{File: "b.go", Kind: entities.KindFunc, Name: "B"}
	result := indexing.Result{
		Graph: entities.NewGraph([]entities.Decl{{Key: declKey()}, {Key: other}}, nil, nil),
		Blind: []indexing.Blind{
			{Decl: other, Query: "implementations", Reason: "timed out"},
			{Decl: declKey(), Query: "references", Reason: "no answer"},
			{Decl: declKey(), Query: "references", Reason: "no answer"},
		},
	}

	WarnShortfalls(result)

	want := `msg="the resolver could not answer for some declarations" count=3 of=2 ` +
		`first=` + other.String() + ` query=implementations reason="timed out"`
	if got := strings.TrimSpace(logged.String()); got != want {
		t.Fatalf("logged %q, want %q", got, want)
	}
}

func TestWarnShortfallsNamesTheCauseOfAPartialPass(t *testing.T) {
	const head = `msg="some packages did not type-check; their type facts are partial" `

	testCases := []struct {
		name    string
		partial indexing.Partial
		want    string
	}{
		{
			name: "the first broken package, its error and how many broken packages carry that message",
			partial: indexing.Partial{
				Broken: 2, Of: 9, FirstPackage: "example.com/p/a",
				FirstError: "reading lifted/go.mod: no such file", SameMessage: 2,
			},
			want: head + `count=2 of=9 tags=[] first=example.com/p/a ` +
				`cause="reading lifted/go.mod: no such file" same_message=2`,
		},
		{
			name:    "a partial with no first package names no cause",
			partial: indexing.Partial{Broken: 2, Of: 9},
			want:    head + `count=2 of=9 tags=[]`,
		},
	}

	for _, testCase := range testCases {
		logged := captureWarnings(t)

		partial := []indexing.Partial{testCase.partial}

		WarnShortfalls(indexing.Result{Graph: declGraph(), Partial: partial})

		if got := strings.TrimSpace(logged.String()); got != testCase.want {
			t.Fatalf("%s: logged %q, want %q", testCase.name, got, testCase.want)
		}
	}
}

// TestWarnUnclaimed pins that a path no language pack claims is logged (not
// dropped), and that nothing is logged when every path was claimed. It runs
// outside t.Parallel() for the same reason TestWarnBlind does: it swaps the
// package-global slog default, and that state is process-wide.
func TestWarnUnclaimed(t *testing.T) {
	testCases := []struct {
		name      string
		unclaimed []auditing.Unclaimed
		wantLines int
	}{
		{"nothing unclaimed logs nothing", nil, 0},
		{
			"an unclaimed path is logged",
			[]auditing.Unclaimed{{File: "database/atlas.hcl"}},
			1,
		},
	}

	for _, testCase := range testCases {
		var buf bytes.Buffer

		restore := slog.Default()
		slog.SetDefault(
			slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})),
		)

		WarnUnclaimed(testCase.unclaimed)

		slog.SetDefault(restore)

		got := 0
		if buf.Len() > 0 {
			got = 1
		}

		if got != testCase.wantLines {
			t.Errorf(
				"%s: logged = %d lines, want %d (output: %q)",
				testCase.name,
				got,
				testCase.wantLines,
				buf.String(),
			)
		}
	}
}
