package anchorprobe

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/checkrun"
)

func TestConfigure(t *testing.T) {
	t.Parallel()

	steps := `{"languages":{"go":{"steps":[{"cmd":["go","build","./..."]}],"env":["GOWORK=off"]}}}`
	testCases := []struct {
		name        string
		tags        []string
		section     string
		wantGo      checkrun.Language
		wantExclude []string
		wantErr     bool
	}{
		{
			name: "no section is the default go", tags: []string{"integration"},
			wantGo: checkrun.GoDefault([]string{"integration"}),
		},
		{
			name: "section tags apply when none are given", section: `{"tags":["slow"]}`,
			wantGo: checkrun.GoDefault([]string{"slow"}),
		},
		{
			name: "given tags win over the section", tags: []string{"fast"},
			section: `{"tags":["slow"]}`, wantGo: checkrun.GoDefault([]string{"fast"}),
		},
		{
			name: "a redefined go brings its own steps and env", section: steps,
			wantGo: checkrun.Language{
				Steps: []checkrun.Step{{Argv: []string{"go", "build", "./..."}}},
				Env:   []string{"GOWORK=off"},
			},
		},
		{
			name: "exclude is carried", section: `{"exclude":["lifted"]}`,
			wantGo: checkrun.GoDefault(nil), wantExclude: []string{"lifted"},
		},
		{name: "an unknown key is refused", section: `{"nope":1}`, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := configure(testCase.tags, json.RawMessage(testCase.section))
			if (err != nil) != testCase.wantErr {
				t.Fatalf("configure() error = %v, wantErr %v", err, testCase.wantErr)
			}

			if testCase.wantErr {
				return
			}

			goLanguage := got.languages["go"]
			sameStep := func(left, right checkrun.Step) bool {
				return slices.Equal(left.Argv, right.Argv) &&
					slices.Equal(left.Tolerate, right.Tolerate)
			}

			if !slices.EqualFunc(goLanguage.Steps, testCase.wantGo.Steps, sameStep) ||
				!slices.Equal(goLanguage.Env, testCase.wantGo.Env) {
				t.Fatalf("go language = %+v, want %+v", goLanguage, testCase.wantGo)
			}

			if !slices.Equal(got.exclude, testCase.wantExclude) || got.markers["go.mod"] != "go" {
				t.Fatalf("exclude = %v markers = %v", got.exclude, got.markers)
			}
		})
	}
}
