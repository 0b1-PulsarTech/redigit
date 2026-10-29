package cli

import (
	"errors"
	"maps"
	"testing"
)

type parseTestCase struct {
	name     string
	args     []string
	wantErr  bool
	wantRepo string
	wantRev  string
	wantTags []string
	wantFar  int
}

func TestParse(t *testing.T) {
	t.Parallel()

	testCases := []parseTestCase{
		{
			name:    "missing --repo is a usage error",
			args:    []string{"--rev", "abc123"},
			wantErr: true,
		},
		{
			name:    "an unknown flag is a usage error",
			args:    []string{"--repo", ".", "--nope", "x"},
			wantErr: true,
		},
		{
			name:     "tags split on commas",
			args:     []string{"--repo", ".", "--tags", "a,b"},
			wantRepo: ".",
			wantRev:  "HEAD",
			wantTags: []string{"a", "b"},
		},
		{
			name:     "an empty tags value yields no tags",
			args:     []string{"--repo", ".", "--tags", ""},
			wantRepo: ".",
			wantTags: nil,
		},
		{
			name:     "whitespace-only tags yield no tags",
			args:     []string{"--repo", ".", "--tags", "  ,  ,"},
			wantRepo: ".",
			wantTags: nil,
		},
		{
			name:     "defaults apply when only --repo is given",
			args:     []string{"--repo", "/x"},
			wantRepo: "/x",
			wantRev:  "HEAD",
			wantFar:  0,
		},
		{
			name:     "--far is threaded through",
			args:     []string{"--repo", "/x", "--far", "12"},
			wantRepo: "/x",
			wantFar:  12,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			checkParseResult(t, testCase)
		})
	}
}

func checkParseResult(t *testing.T, testCase parseTestCase) {
	t.Helper()

	cfg, far, err := parse(testCase.args)

	if testCase.wantErr {
		if !errors.Is(err, ErrUsage) {
			t.Fatalf("err = %v, want it to wrap ErrUsage", err)
		}

		return
	}

	if err != nil {
		t.Fatalf("parse() = %v", err)
	}

	if cfg.Repo != testCase.wantRepo {
		t.Fatalf("Repo = %q, want %q", cfg.Repo, testCase.wantRepo)
	}

	if testCase.wantRev != "" && cfg.Rev != testCase.wantRev {
		t.Fatalf("Rev = %q, want %q", cfg.Rev, testCase.wantRev)
	}

	if len(cfg.BuildTags) != len(testCase.wantTags) {
		t.Fatalf("BuildTags = %v, want %v", cfg.BuildTags, testCase.wantTags)
	}

	for i, tag := range testCase.wantTags {
		if cfg.BuildTags[i] != tag {
			t.Fatalf("BuildTags = %v, want %v", cfg.BuildTags, testCase.wantTags)
		}
	}

	if far != testCase.wantFar {
		t.Fatalf("far = %d, want %d", far, testCase.wantFar)
	}
}

func TestSplitTags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		tags string
		want []string
	}{
		{"empty string yields none", "", nil},
		{"whitespace only yields none", "   ", nil},
		{"one tag", "a", []string{"a"}},
		{"two tags", "a,b", []string{"a", "b"}},
		{"blank entries between commas are dropped", "a,,b", []string{"a", "b"}},
		{"surrounding whitespace is trimmed", " a , b ", []string{"a", "b"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := splitTags(testCase.tags)

			if len(got) != len(testCase.want) {
				t.Fatalf("splitTags(%q) = %v, want %v", testCase.tags, got, testCase.want)
			}

			for i, tag := range testCase.want {
				if got[i] != tag {
					t.Fatalf("splitTags(%q) = %v, want %v", testCase.tags, got, testCase.want)
				}
			}
		})
	}
}

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
