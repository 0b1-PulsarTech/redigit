package cli

import (
	"errors"
	"maps"
	"testing"
)

func TestParseVirtual(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		args    []string
		want    map[string]string
		wantErr bool
	}{
		{"no flag maps nothing", []string{"--repo", "."}, nil, false},
		{
			"the flag repeats",
			[]string{
				"--repo", ".",
				"--virtual", "virtual:a=web/tool/a.dart",
				"--virtual", " virtual:b = web/tool/b.dart ",
			},
			map[string]string{"virtual:a": "web/tool/a.dart", "virtual:b": "web/tool/b.dart"},
			false,
		},
		{
			"a value without = is a usage error",
			[]string{"--repo", ".", "--virtual", "virtual:a"},
			nil,
			true,
		},
		{
			"an empty path is a usage error",
			[]string{"--repo", ".", "--virtual", "virtual:a="},
			nil,
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg, _, err := parse(testCase.args)

			if testCase.wantErr {
				if !errors.Is(err, ErrUsage) {
					t.Fatalf("err = %v, want it to wrap ErrUsage", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("parse() = %v", err)
			}

			if !maps.Equal(cfg.Virtual, testCase.want) {
				t.Fatalf("Virtual = %v, want %v", cfg.Virtual, testCase.want)
			}
		})
	}
}
