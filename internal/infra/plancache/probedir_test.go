package plancache

import (
	"errors"
	"os"
	"testing"
)

func TestRefuseOffRepo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		file    string
		content string
		wantErr bool
	}{
		{"replace outside", "go.mod", "module x\nreplace example.com/a => ../x\n", true},
		{"replace absolute", "go.mod", "module x\nreplace example.com/a => /abs/x\n", true},
		{"replace inside", "go.mod", "module x\nreplace example.com/a => ./x\n", false},
		{
			"replace remote",
			"go.mod",
			"module x\nreplace example.com/a => example.com/x v1.0.0\n",
			false,
		},
		{"replace in a block", "go.mod", "module x\nreplace (\n\texample.com/a => ../x\n)\n", true},
		{"unparsable go.mod", "go.mod", "module \"x\n", true},
		{"no replace", "go.mod", "module x\n", false},
		{"use outside", "go.work", "go 1.25\nuse ../x\n", true},
		{"use absolute", "go.work", "go 1.25\nuse /abs\n", true},
		{"use inside", "go.work", "go 1.25\nuse ./a\n", false},
		{"use the root", "go.work", "go 1.25\nuse .\n", false},
		{"work replace outside", "go.work", "go 1.25\nreplace example.com/a => ../x\n", true},
		{"work replace inside", "go.work", "go 1.25\nreplace example.com/a => ./x\n", false},
		{
			"work replace remote",
			"go.work",
			"go 1.25\nreplace example.com/a => example.com/x v1.0.0\n",
			false,
		},
		{"unparsable go.work", "go.work", "go 1.25\nuse\n", true},
		{
			"replace rooted by a backslash",
			"go.mod",
			"module x\nreplace example.com/a => \\abs\n",
			true,
		},
		{"use rooted by a backslash", "go.work", "go 1.25\nuse \\abs\n", true},
		{
			"nested replace escapes",
			"svc/go.mod",
			"module x\nreplace example.com/a => ../../lib\n",
			true,
		},
		{
			"nested replace stays in",
			"svc/go.mod",
			"module x\nreplace example.com/a => ../lib\n",
			false,
		},
		{
			"deep replace stays in",
			"a/b/go.mod",
			"module x\nreplace example.com/a => ../../x\n",
			false,
		},
		{
			"deep replace escapes",
			"a/b/go.mod",
			"module x\nreplace example.com/a => ../../../x\n",
			true,
		},
		{"unparsable nested go.mod", "svc/go.mod", "module \"x\n", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := refuseOffRepo(testCase.file, []byte(testCase.content))
			if testCase.wantErr != errors.Is(err, ErrToolchain) {
				t.Fatalf("refuseOffRepo() err = %v, want refusal = %v", err, testCase.wantErr)
			}
		})
	}
}

func TestModuleProbeDirRefusesWhatLeavesTheRepo(t *testing.T) {
	testCases := []struct {
		name    string
		files   map[string]string
		wantErr bool
	}{
		{
			"go.mod replace outside",
			map[string]string{"go.mod": "module x\nreplace example.com/a => ../x\n"},
			true,
		},
		{
			"go.work use outside",
			map[string]string{
				"go.mod":  "module x\n",
				"go.work": "go 1.25\nuse ../x\n",
			},
			true,
		},
		{
			"nested go.mod replace outside",
			map[string]string{
				"go.mod":     "module x\n",
				"svc/go.mod": "module y\nreplace example.com/a => ../../lib\n",
			},
			true,
		},
		{
			"unparsable nested go.mod",
			map[string]string{
				"go.mod":     "module x\n",
				"svc/go.mod": "module \"y\n",
			},
			true,
		},
		{
			"nested go.mod replace inside",
			map[string]string{
				"go.mod":     "module x\n",
				"a/b/go.mod": "module y\nreplace example.com/a => ../../lib\n",
			},
			false,
		},
		{
			"all inside",
			map[string]string{
				"go.mod":  "module x\nreplace example.com/a => ./x\n",
				"go.work": "go 1.25\nuse ./a\n",
			},
			false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			repo, commit := newRepo(t, testCase.files)

			dir, err := moduleProbeDir(t.Context(), repo, commit)
			if testCase.wantErr != errors.Is(err, ErrToolchain) {
				t.Fatalf("moduleProbeDir() err = %v, want refusal = %v", err, testCase.wantErr)
			}

			if err == nil {
				_ = os.RemoveAll(dir)
			}
		})
	}
}
