package format

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type (
	parityRow struct {
		Position int `json:"position"`
		Blobs    []struct {
			Path   string `json:"path"`
			Handle string `json:"handle"`
		} `json:"blobs"`
	}

	parityPair struct {
		position int
		path     string
		raw, fmt string
	}

	catFile struct {
		stdin  io.WriteCloser
		stdout *bufio.Reader
		cmd    *exec.Cmd
	}
)

func startCatFile(t *testing.T, repo string) *catFile {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", "-C", repo, "cat-file", "--batch")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	return &catFile{stdin: stdin, stdout: bufio.NewReaderSize(stdout, 1<<20), cmd: cmd}
}

func (cat *catFile) blob(t *testing.T, handle string) []byte {
	t.Helper()

	if _, err := fmt.Fprintln(cat.stdin, handle); err != nil {
		t.Fatal(err)
	}

	header, err := cat.stdout.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	fields := strings.Fields(header)
	if len(fields) != 3 {
		t.Fatalf("blob %s: %q", handle, header)
	}

	size, _ := strconv.Atoi(fields[2])
	body := make([]byte, size+1)

	if _, err = io.ReadFull(cat.stdout, body); err != nil {
		t.Fatal(err)
	}

	return body[:size]
}

func readParityRows(t *testing.T, file string) map[int]map[string]string {
	t.Helper()

	handle, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()

	out := map[int]map[string]string{}
	reader := bufio.NewReaderSize(handle, 1<<20)

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var row parityRow
			if jsonErr := json.Unmarshal(line, &row); jsonErr != nil {
				t.Fatal(jsonErr)
			}

			paths := map[string]string{}

			for _, blob := range row.Blobs {
				if strings.HasSuffix(blob.Path, ".go") {
					paths[blob.Path] = blob.Handle
				}
			}

			out[row.Position] = paths
		}

		if readErr != nil {
			return out
		}
	}
}

func squash(content []byte) string {
	return strings.Join(strings.Fields(string(content)), "")
}

func lines(content []byte) []string {
	return strings.Split(string(content), "\n")
}

func classify(ours, theirs []byte) (kind, example string) {
	kind = "other"
	if squash(ours) == squash(theirs) {
		kind = "line wrapping"
	}

	ourLines, theirLines := lines(ours), lines(theirs)
	inImports := false

	for index := 0; index < min(len(ourLines), len(theirLines)); index++ {
		inImports = inImports || strings.HasPrefix(ourLines[index], "import (")

		if ourLines[index] != theirLines[index] {
			if inImports && kind != "other" {
				kind = "import grouping"
			}

			return kind, fmt.Sprintf("ours:   %q\ntheirs: %q", ourLines[index], theirLines[index])
		}

		inImports = inImports && ourLines[index] != ")"
	}

	return kind, "tail differs"
}

type (
	loadedPair struct{ raw, want []byte }

	parityTally struct {
		counts   map[string]int
		examples map[string][]string
	}
)

func (tally parityTally) add(kind, example string) {
	tally.counts[kind]++

	const maxExamples = 3

	if len(tally.examples[kind]) < maxExamples {
		tally.examples[kind] = append(tally.examples[kind], example)
	}
}

func loadPairs(t *testing.T, dir, repo string) ([]parityPair, []loadedPair) {
	t.Helper()

	raw := readParityRows(t, filepath.Join(dir, "r21k_x0.jsonl"))
	formatted := readParityRows(t, filepath.Join(dir, "r21k_x0-fmt.jsonl"))

	var pairs []parityPair

	for position, paths := range raw {
		for file, handle := range paths {
			if other, found := formatted[position][file]; found {
				pairs = append(pairs, parityPair{position, file, handle, other})
			}
		}
	}

	cat := startCatFile(t, repo)
	defer func() { _ = cat.cmd.Process.Kill() }()

	blobs := make([]loadedPair, len(pairs))
	for index, pair := range pairs {
		blobs[index] = loadedPair{cat.blob(t, pair.raw), cat.blob(t, pair.fmt)}
	}

	return pairs, blobs
}

func TestParityAgainstRetiredStage(t *testing.T) {
	dir, repo := os.Getenv("REDIGIT_PARITY_DIR"), os.Getenv("REDIGIT_TERECTEK")
	if dir == "" || repo == "" {
		t.Skip("REDIGIT_PARITY_DIR and REDIGIT_TERECTEK are not set")
	}

	pairs, blobs := loadPairs(t, dir, repo)
	engine := parityEngine(t, repo)

	inputs := make([]File, len(pairs))
	for index, pair := range pairs {
		inputs[index] = File{Path: pair.path, Content: blobs[index].raw}
	}

	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	start := time.Now()
	outputs, failures := engine.FormatAll(context.Background(), inputs)
	wall := time.Since(start)

	runtime.ReadMemStats(&after)

	tally := parityTally{counts: map[string]int{}, examples: map[string][]string{}}
	failedPath := map[string]bool{}

	for _, failure := range failures {
		failedPath[failure.Path] = true

		tally.add("tool failure", failure.Error())
	}

	identical, pythonChanged, ourChanged := tally.compare(pairs, blobs, outputs, failedPath)

	t.Logf("pythonChanged=%d ourChanged=%d", pythonChanged, ourChanged)
	t.Logf("pairs=%d identical=%d failures=%d wall=%s allocs=%d bytes=%d",
		len(pairs), identical, len(failures), wall,
		after.Mallocs-before.Mallocs, after.TotalAlloc-before.TotalAlloc)

	for kind, count := range tally.counts {
		t.Logf("class %q: %d", kind, count)

		for _, example := range tally.examples[kind] {
			t.Logf("  example %s", example)
		}
	}

	if wall > time.Minute {
		t.Errorf("formatting took %s, budget 60s", wall)
	}
}

func parityEngine(t *testing.T, repo string) *Engine {
	t.Helper()

	for _, name := range []string{"gofumpt", "goimports", "golines"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Skipf("%s is not on PATH", name)
		}

		version, _ := exec.CommandContext(t.Context(), "go", "version", "-m", path).CombinedOutput()

		t.Logf("%s", strings.Join(strings.Split(string(version), "\n")[:3], " | "))
	}

	gomod, err := exec.CommandContext(t.Context(), "git", "-C", repo, "cat-file", "blob", "HEAD:libs/tereckernel/go.mod").
		Output()
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	if err = os.WriteFile(filepath.Join(root, "go.mod"), gomod, 0o600); err != nil {
		t.Fatal(err)
	}

	timeout := Duration(2 * time.Minute)

	engine, err := New(Config{
		ConfigRoot: root,
		CopyFiles:  []string{"go.mod"},
		Languages: []Language{{
			Name:       "go",
			Extensions: []string{".go"},
			SkipMarker: "Code generated",
			Steps:      paritySteps(timeout),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	return engine
}

func paritySteps(timeout Duration) []Step {
	all := map[string]Step{
		"gofumpt":   {Cmd: []string{"gofumpt", "-w"}, Mode: ModeFiles, Timeout: timeout},
		"goimports": {Cmd: []string{"goimports", "-w"}, Mode: ModeFiles, Timeout: timeout},
		"imports-format-only": {
			Cmd: []string{"goimports", "-w", "-format-only"}, Mode: ModeFiles, Timeout: timeout,
		},
		"golines": {
			Cmd: []string{
				"golines",
				"-w",
				"-m",
				"100",
			},
			Mode:    ModeFiles,
			Timeout: timeout,
			IfWider: 100,
		},
	}

	names := strings.Split(os.Getenv("REDIGIT_PARITY_STEPS"), ",")
	if os.Getenv("REDIGIT_PARITY_STEPS") == "" {
		names = []string{"gofumpt", "goimports", "golines"}
	}

	steps := make([]Step, 0, len(names))
	for _, name := range names {
		steps = append(steps, all[name])
	}

	return steps
}

func (tally parityTally) compare(
	pairs []parityPair,
	blobs []loadedPair,
	outputs []File,
	failedPath map[string]bool,
) (identical, pythonChanged, ourChanged int) {
	for index, pair := range pairs {
		got := outputs[index].Content

		if pair.raw != pair.fmt {
			pythonChanged++
		}

		if !bytes.Equal(got, blobs[index].raw) {
			ourChanged++
		}

		switch {
		case bytes.Equal(got, blobs[index].want):
			identical++
		case failedPath[pair.path]:
			continue
		default:
			kind, example := classify(got, blobs[index].want)
			tally.add(kind, pair.path+"\n"+example)
		}
	}

	return identical, pythonChanged, ourChanged
}
