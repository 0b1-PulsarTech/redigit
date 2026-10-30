package verify

import (
	"errors"
	"reflect"
	"runtime"
	"strconv"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		args    []string
		want    options
		wantErr error
	}{
		{
			"defaults", nil,
			options{
				repo: ".", ref: "HEAD", stride: 1, parallel: min(2, runtime.NumCPU()),
				provided: map[string]bool{},
			},
			nil,
		},
		{
			"every flag",
			[]string{
				"--repo", "r", "--ref", "refs/heads/x", "--from", "3", "--to", "9",
				"--stride", "4", "--parallel", "3", "--tags", "a,b", "--exclude", "x,y",
				"--config", "c.json", "--cache-dir", "cd",
			},
			options{
				repo: "r", ref: "refs/heads/x", from: 3, to: 9, stride: 4, parallel: 3,
				tags: "a,b", exclude: "x,y", configPath: "c.json", cacheDir: "cd",
				provided: map[string]bool{
					"repo": true, "ref": true, "from": true, "to": true, "stride": true,
					"parallel": true, "tags": true, "exclude": true, "config": true,
					"cache-dir": true,
				},
			},
			nil,
		},
		{"unknown flag", []string{"--nope"}, options{}, ErrUsage},
		{"positional argument", []string{"extra"}, options{}, ErrUsage},
		{"flag without a value", []string{"--repo"}, options{}, ErrUsage},
		{"stride not a number", []string{"--stride", "x"}, options{}, ErrUsage},
		{"stride zero", []string{"--stride", "0"}, options{}, ErrUsage},
		{"parallel zero", []string{"--parallel", "0"}, options{}, ErrUsage},
		{
			"parallel above the cpu count",
			[]string{"--parallel", strconv.Itoa(runtime.NumCPU() + 1)},
			options{},
			ErrUsage,
		},
		{"from negative", []string{"--from", "-1"}, options{}, ErrUsage},
		{"to negative", []string{"--to", "-1"}, options{}, ErrUsage},
		{"from after to", []string{"--from", "5", "--to", "2"}, options{}, ErrUsage},
		{"ref starts with a dash", []string{"--ref", "-x"}, options{}, ErrUsage},
		{"empty repo", []string{"--repo", ""}, options{}, ErrUsage},
		{"exclude of the root", []string{"--exclude", "."}, options{}, ErrUsage},
		{"exclude of the root beside a dir", []string{"--exclude", "a,./"}, options{}, ErrUsage},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parse(testCase.args)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("parse() err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr == nil && !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("parse() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
