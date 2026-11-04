package verifying

import (
	"errors"
	"slices"
	"testing"
)

func commitsOf(count int) []Commit {
	commits := make([]Commit, count)
	for idx := range commits {
		commits[idx] = Commit{Oid: string(rune('a' + idx)), Position: idx + 1}
	}

	return commits
}

func positionsOf(commits []Commit) []int {
	positions := make([]int, 0, len(commits))
	for _, commit := range commits {
		positions = append(positions, commit.Position)
	}

	return positions
}

func TestIsExcluded(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		dir      string
		prefixes []string
		want     bool
	}{
		{"the exact dir", "m", []string{"m"}, true},
		{"a trailing slash", "m", []string{"m/"}, true},
		{"a leading dot slash", "m", []string{"./m"}, true},
		{"a trailing dot", "m", []string{"m/."}, true},
		{"doubled slashes", "m", []string{"m//"}, true},
		{"a nested dir under a dotted prefix", "m/sub", []string{"./m/"}, true},
		{"a dotted dir against a plain prefix", "./m", []string{"m"}, true},
		{"a sibling sharing a prefix", "mx", []string{"./m"}, false},
		{"another dir", "other", []string{"./m", "n/"}, false},
		{"no prefixes", "m", nil, false},
		{"an empty prefix", "m", []string{""}, false},
		{"an empty prefix against the root", ".", []string{""}, false},
		{"a dot prefix", "m", []string{"."}, false},
		{"a dot prefix against the root", ".", []string{"./"}, false},
		{"a bare prefix beside a real one", "m", []string{"", "m"}, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := IsExcluded(testCase.dir, testCase.prefixes); got != testCase.want {
				t.Fatalf("IsExcluded(%q, %q) = %v, want %v",
					testCase.dir, testCase.prefixes, got, testCase.want)
			}
		})
	}
}

func TestSelect(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		count   int
		opts    Options
		want    []int
		wantErr error
	}{
		{"every commit by default", 4, Options{}, []int{1, 2, 3, 4}, nil},
		{"stride keeps the tip", 7, Options{Stride: 3}, []int{1, 4, 7}, nil},
		{
			"stride adds the tip when it is off the grid",
			8,
			Options{Stride: 3},
			[]int{1, 4, 7, 8},
			nil,
		},
		{"from and to bound the range", 9, Options{From: 3, To: 6}, []int{3, 4, 5, 6}, nil},
		{
			"stride counts from the first selected",
			9,
			Options{From: 2, To: 8, Stride: 3},
			[]int{2, 5, 8},
			nil,
		},
		{"to past the end clamps", 3, Options{To: 99}, []int{1, 2, 3}, nil},
		{"from at the last commit", 3, Options{From: 3}, []int{3}, nil},
		{"from past the end is invalid", 3, Options{From: 9}, nil, ErrInvalid},
		{"no commits", 0, Options{}, []int{}, nil},
		{"negative stride", 3, Options{Stride: -1}, nil, ErrInvalid},
		{"negative parallel", 3, Options{Parallel: -1}, nil, ErrInvalid},
		{"negative from", 3, Options{From: -1}, nil, ErrInvalid},
		{"from after to", 5, Options{From: 4, To: 2}, nil, ErrInvalid},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Select(commitsOf(testCase.count), testCase.opts)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantErr != nil {
				return
			}

			if positions := positionsOf(got); !slices.Equal(positions, testCase.want) {
				t.Fatalf("positions = %v, want %v", positions, testCase.want)
			}
		})
	}
}
