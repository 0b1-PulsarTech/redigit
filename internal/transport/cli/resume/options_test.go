package resume

import (
	"errors"
	"maps"
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/anchorstate"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

func TestParseOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		args    []string
		want    options
		wantErr error
	}{
		{
			name: "decisions and a rest",
			args: []string{"--state", "dir", "--fold", "aa", "--wip", "bb", "--rest", "wip"},
			want: options{
				state: "dir", rest: anchoring.PolicyWIP,
				folds: []string{"aa"}, wips: []string{"bb"},
			},
		},
		{
			name: "a rest alone",
			args: []string{"--state", "dir", "--rest", "fold"},
			want: options{state: "dir", rest: anchoring.PolicyFold},
		},
		{
			name: "abort alone",
			args: []string{"--state", "dir", "--abort"},
			want: options{state: "dir", isAbort: true},
		},
		{name: "no state", args: []string{"--abort"}, wantErr: write.ErrUsage},
		{
			name:    "an unknown rest",
			args:    []string{"--state", "dir", "--rest", "stop"},
			wantErr: write.ErrUsage,
		},
		{
			name:    "abort with a decision",
			args:    []string{"--state", "dir", "--abort", "--fold", "aa"},
			wantErr: write.ErrUsage,
		},
		{name: "nothing to do", args: []string{"--state", "dir"}, wantErr: write.ErrUsage},
		{
			name:    "an unknown flag",
			args:    []string{"--state", "dir", "--no"},
			wantErr: write.ErrUsage,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseOptions(testCase.args)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("parseOptions() error = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("parseOptions() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestDecisionsOf(t *testing.T) {
	t.Parallel()

	state := anchorstate.State{Conflicts: []anchoring.Conflict{
		{Rev: "aa11"}, {Rev: "aa22"}, {Rev: "bb33"},
	}}

	testCases := []struct {
		name    string
		opts    options
		want    map[string]anchoring.Policy
		wantErr error
		isLone  bool
	}{
		{name: "nothing decided", opts: options{}, want: map[string]anchoring.Policy{}},
		{
			name: "an empty rev against a single conflict", opts: options{wips: []string{""}},
			wantErr: write.ErrUsage, isLone: true,
		},
		{
			name:    "a rev longer than any conflict",
			opts:    options{folds: []string{"aa11223"}},
			wantErr: write.ErrUsage,
		},
		{
			name: "prefixes name full revs",
			opts: options{folds: []string{"aa1"}, wips: []string{"bb"}},
			want: map[string]anchoring.Policy{
				"aa11": anchoring.PolicyFold, "bb33": anchoring.PolicyWIP,
			},
		},
		{
			name:    "an ambiguous prefix",
			opts:    options{folds: []string{"aa"}},
			wantErr: write.ErrUsage,
		},
		{name: "an unknown rev", opts: options{folds: []string{"cc"}}, wantErr: write.ErrUsage},
		{name: "an empty rev", opts: options{folds: []string{""}}, wantErr: write.ErrUsage},
		{
			name:    "one anchor decided twice",
			opts:    options{folds: []string{"bb"}, wips: []string{"bb3"}},
			wantErr: write.ErrUsage,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			current := state
			if testCase.isLone {
				current = anchorstate.State{Conflicts: []anchoring.Conflict{{Rev: "aa11"}}}
			}

			got, err := decisionsOf(testCase.opts, current)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("decisionsOf() error = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && !maps.Equal(got, testCase.want) {
				t.Fatalf("decisionsOf() = %v, want %v", got, testCase.want)
			}
		})
	}
}
