package plancache

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
)

func baseInputs() Inputs {
	return Inputs{
		BuildID:    "build-1",
		Commit:     "commit-1",
		Server:     "server-1",
		ServerArgs: []string{"-mode=stdio"},
		Toolchain:  "go-1",
		Virtual:    map[string]string{"virtual:x": "web/x.ts"},
		Flags: []Flag{
			{Name: "tags", Value: "integration"},
			{Name: "repo", Value: "/src"},
		},
	}
}

func mustKey(t *testing.T, inputs Inputs) string {
	t.Helper()

	key, err := inputs.Key()
	if err != nil {
		t.Fatalf("Key() err = %v", err)
	}

	return key
}

func TestKeyChangesWithEveryInput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(*Inputs)
	}{
		{"build id", func(in *Inputs) { in.BuildID = "build-2" }},
		{"commit", func(in *Inputs) { in.Commit = "commit-2" }},
		{"server digest", func(in *Inputs) { in.Server = "server-2" }},
		{"server args", func(in *Inputs) { in.ServerArgs = []string{"-mode=stdio", "-rpc.trace"} }},
		{"toolchain digest", func(in *Inputs) { in.Toolchain = "go-2" }},
		{
			"virtual module value",
			func(in *Inputs) { in.Virtual = map[string]string{"virtual:x": "web/y.ts"} },
		},
		{
			"virtual module id",
			func(in *Inputs) { in.Virtual = map[string]string{"virtual:z": "web/x.ts"} },
		},
		{"no virtual modules", func(in *Inputs) { in.Virtual = nil }},
		{"flag value", func(in *Inputs) { in.Flags[0].Value = "other" }},
		{"flag name", func(in *Inputs) { in.Flags[0].Name = "rev" }},
		{
			"extra flag",
			func(in *Inputs) { in.Flags = append(in.Flags, Flag{Name: "far", Value: "3"}) },
		},
		{"fields shifted across a boundary", func(in *Inputs) {
			in.Commit, in.Server = "commit-1s", "erver-1"
		}},
	}

	want := mustKey(t, baseInputs())

	if len(want) != 64 {
		t.Fatalf("Key() = %q, want 64 hex digits", want)
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			inputs := baseInputs()
			testCase.mutate(&inputs)

			if got := mustKey(t, inputs); got == want {
				t.Fatalf("Key() did not change when the %s changed", testCase.name)
			}
		})
	}
}

func TestKeyIgnoresFlagOrder(t *testing.T) {
	t.Parallel()

	forward := baseInputs()
	backward := baseInputs()
	backward.Flags = []Flag{forward.Flags[1], forward.Flags[0]}

	if mustKey(t, forward) != mustKey(t, backward) {
		t.Fatal("Key() depends on the order flags were set in")
	}

	if forward.Flags[0].Name != "tags" {
		t.Fatal("Key() reordered the caller's flags")
	}
}

func TestKeyHashesFileFlagBytes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	layers := writeFile(t, dir, "layers", "1 libs\n", 0o600)
	copied := writeFile(t, dir, "copy", "1 libs\n", 0o600)
	edited := writeFile(t, dir, "edited", "2 libs\n", 0o600)

	withFile := func(path string) Inputs {
		inputs := baseInputs()
		inputs.Flags = append(inputs.Flags, Flag{Name: "layers", Value: path, IsFile: true})

		return inputs
	}

	if mustKey(t, withFile(layers)) != mustKey(t, withFile(copied)) {
		t.Fatal("Key() depends on the path of a file flag, not its bytes")
	}

	if mustKey(t, withFile(layers)) == mustKey(t, withFile(edited)) {
		t.Fatal("Key() did not change when the file's bytes changed")
	}

	if mustKey(t, withFile(layers)) == mustKey(t, withFile("")) {
		t.Fatal("Key() did not tell a set file flag from an empty one")
	}

	asValue := baseInputs()
	asValue.Flags = append(asValue.Flags, Flag{Name: "layers", Value: layers})

	if mustKey(t, withFile(layers)) == mustKey(t, asValue) {
		t.Fatal("Key() did not tell a file flag from a value flag")
	}
}

func TestKeyFailsOnAnUnreadableFileFlag(t *testing.T) {
	t.Parallel()

	inputs := baseInputs()
	inputs.Flags = append(inputs.Flags, Flag{
		Name:   "features",
		Value:  filepath.Join(t.TempDir(), "absent"),
		IsFile: true,
	})

	if key, err := inputs.Key(); err == nil || key != "" {
		t.Fatalf("Key() = %q, %v, want an error and no key", key, err)
	}
}

func TestKeyUsesFileFlagContentWithoutOpeningThePath(t *testing.T) {
	t.Parallel()

	withContent := func(path, content string) Inputs {
		inputs := baseInputs()
		inputs.Flags = append(inputs.Flags, Flag{
			Name: "config", Value: path, IsFile: true, Content: []byte(content),
		})

		return inputs
	}

	absent := filepath.Join(t.TempDir(), "absent")

	first := mustKey(t, withContent(absent, "a"))

	if mustKey(t, withContent(absent, "a")) != first {
		t.Fatal("Key() is not stable for the same content")
	}

	if mustKey(t, withContent(absent, "b")) == first {
		t.Fatal("Key() did not change when the content changed under one path")
	}

	if mustKey(t, withContent(absent+"2", "a")) != first {
		t.Fatal("Key() depends on the path of a flag that carries its content")
	}
}

func TestPutLengthPrefixesEveryField(t *testing.T) {
	t.Parallel()

	sumOf := func(fields ...string) string {
		hasher := sha256.New()
		put(hasher, fields...)

		return hex.EncodeToString(hasher.Sum(nil))
	}

	if sumOf("ab", "c") == sumOf("a", "bc") {
		t.Fatal("put ran two fields together: ab+c hashes as a+bc")
	}
}

func TestKeyFormatIsPinned(t *testing.T) {
	t.Parallel()

	const empty = "b987075bf03cfafb8c492c5ab58c02db38b0b07643b579f837891e5740c34111"

	if got := mustKey(t, Inputs{}); got != empty {
		t.Fatalf("Key() of no inputs = %s, want %s: bump formatVersion with the layout", got, empty)
	}
}
