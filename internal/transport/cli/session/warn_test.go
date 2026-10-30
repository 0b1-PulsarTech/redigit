package session

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
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
