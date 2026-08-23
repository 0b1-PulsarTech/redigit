//go:build integration

package emitting

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// diskSource reads a path's final content straight off disk, and reports it
// executable when its mode bits say so. It is a real adapter, not a fixture:
// nothing about it is fabricated to fit the test.
type diskSource struct{ root string }

func (s diskSource) Read(_ context.Context, path string) ([]byte, FileMode, error) {
	full := filepath.Join(s.root, filepath.FromSlash(path))

	content, err := os.ReadFile(full)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", full, err)
	}

	info, err := os.Stat(full)
	if err != nil {
		return nil, "", fmt.Errorf("stat %s: %w", full, err)
	}

	mode := FileMode("100644")
	if info.Mode()&0o111 != 0 {
		mode = FileMode("100755")
	}

	return content, mode, nil
}

// spanRenderer cuts real byte spans out of a file's real content, in the
// file's own order, so keeping every declaration reproduces the file byte for
// byte and keeping none produces nothing at all.
type spanRenderer struct {
	spans map[entities.DeclKey]entities.Span
}

func (r spanRenderer) Render(
	_ context.Context,
	source []byte,
	keep []entities.DeclKey,
) ([]byte, error) {
	ordered := slices.Clone(keep)
	slices.SortFunc(ordered, func(a, b entities.DeclKey) int {
		return cmp.Compare(r.spans[a].Start, r.spans[b].Start)
	})

	var out bytes.Buffer
	for _, k := range ordered {
		span := r.spans[k]
		out.Write(source[span.Start:span.End])
	}

	return out.Bytes(), nil
}

// contentStore is a real, if tiny, content-addressed object database: the
// same shape section 5 of the design doc asks the final tool to write to a
// git object store, minus git itself.
type contentStore struct{ blobs map[string][]byte }

func (s *contentStore) Write(_ context.Context, content []byte, _ FileMode) (Handle, error) {
	sum := sha256.Sum256(content)
	id := hex.EncodeToString(sum[:])
	s.blobs[id] = content

	return Handle(id), nil
}

// declFile writes one real file under root holding one declaration per line,
// and returns its content alongside the exact byte span each line occupies -
// the same information a real syntax reader would report.
func declFile(
	t *testing.T,
	root, path string,
	names []string,
) ([]byte, map[entities.DeclKey]entities.Span) {
	t.Helper()

	var content bytes.Buffer

	spans := make(map[entities.DeclKey]entities.Span, len(names))

	for _, name := range names {
		line := fmt.Sprintf("func %s() {}\n", name)
		start := content.Len()
		content.WriteString(line)
		spans[key(path, name)] = entities.Span{Start: start, End: content.Len()}
	}

	full := filepath.Join(root, path)
	if err := os.WriteFile(full, content.Bytes(), 0o600); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}

	return content.Bytes(), spans
}

// TestEmitterReproducesRealFilesEndToEnd builds a plan by hand from real files
// on disk - 3 files, 10 declarations, 4 commits - with two forward references
// planted on purpose, and proves the three guarantees this package exists
// for: nothing is lost, nothing is misplaced, and every forward reference the
// tool can see without compiling is reported, and only those.
func TestEmitterReproducesRealFilesEndToEnd(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	file1, spans1 := declFile(t, root, "file1.go", []string{"A", "B", "C", "D"})
	file2, spans2 := declFile(t, root, "file2.go", []string{"E", "F", "G"})
	file3, spans3 := declFile(t, root, "file3.go", []string{"H", "I", "J"})

	spans := make(map[entities.DeclKey]entities.Span)
	for k, v := range spans1 {
		spans[k] = v
	}
	for k, v := range spans2 {
		spans[k] = v
	}
	for k, v := range spans3 {
		spans[k] = v
	}

	a := key("file1.go", "A")
	b := key("file1.go", "B")
	c := key("file1.go", "C")
	d := key("file1.go", "D")
	e := key("file2.go", "E")
	f := key("file2.go", "F")
	g := key("file2.go", "G")
	h := key("file3.go", "H")
	i := key("file3.go", "I")
	j := key("file3.go", "J")

	decls := make([]entities.Decl, 0, len(spans))
	for k, span := range spans {
		decls = append(decls, decl(k, span))
	}

	// Two forward references, planted on purpose: B (commit 0) reads E (born
	// commit 1), and H (commit 2) reads J (born commit 3). Everything else
	// either stays inside its own commit or reads something already born, so
	// it must NOT show up as a risk.
	edges := []entities.Edge{
		{Dependent: c, Provider: b, Reason: entities.ReasonReference}, // same commit
		{Dependent: d, Provider: a, Reason: entities.ReasonReference}, // earlier commit
		{Dependent: b, Provider: e, Reason: entities.ReasonReference}, // forward: planted risk
		{Dependent: g, Provider: d, Reason: entities.ReasonReference}, // earlier commit
		{Dependent: i, Provider: f, Reason: entities.ReasonReference}, // earlier commit
		{Dependent: h, Provider: j, Reason: entities.ReasonReference}, // forward: planted risk
	}

	graph := entities.NewGraph(decls, edges, nil)

	plan := entities.Plan{Commits: []entities.PlannedCommit{
		{Position: 0, Subject: "feat: seed A-C", Decls: []entities.DeclKey{a, b, c}},
		{Position: 1, Subject: "feat: grow D, add E-F", Decls: []entities.DeclKey{d, e, f}},
		{Position: 2, Subject: "feat: close file2, add H-I", Decls: []entities.DeclKey{g, h, i}},
		{Position: 3, Subject: "feat: close file3", Decls: []entities.DeclKey{j}},
	}}

	store := &contentStore{blobs: map[string][]byte{}}

	commits, err := New(diskSource{root: root}, spanRenderer{spans: spans}, store).
		Emit(t.Context(), plan, graph)
	if err != nil {
		t.Fatalf("Emit() = %v", err)
	}

	t.Logf("materialised %d commits, %d files, %d declarations", len(commits), 3, len(spans))

	seenDecls := map[entities.DeclKey]int{}
	lastBlob := map[string]Handle{}
	risks := 0

	for _, commit := range commits {
		if commit.IsEmpty() {
			t.Fatalf("commit %d is empty", commit.Position)
		}

		for _, chunk := range commit.Chunks {
			seenDecls[chunk.Key]++
		}

		for _, blob := range commit.Blobs {
			lastBlob[blob.Path] = blob.Handle
		}

		risks += len(commit.Risks)
	}

	t.Logf("risks reported: %d", risks)

	for k := range spans {
		if seenDecls[k] != 1 {
			t.Fatalf("declaration %s appears in %d commits, want exactly 1", k, seenDecls[k])
		}
	}

	originals := map[string][]byte{"file1.go": file1, "file2.go": file2, "file3.go": file3}
	for path, want := range originals {
		handle, found := lastBlob[path]
		if !found {
			t.Fatalf("%s never got a final blob", path)
		}

		got := store.blobs[string(handle)]
		if !bytes.Equal(got, want) {
			t.Fatalf(
				"%s final blob does not reproduce the original byte for byte\ngot:  %q\nwant: %q",
				path,
				got,
				want,
			)
		}
	}

	wantRisks := []Risk{
		{Chunk: b, Target: e, BornAt: 1},
		{Chunk: h, Target: j, BornAt: 3},
	}

	var gotRisks []Risk
	for _, commit := range commits {
		gotRisks = append(gotRisks, commit.Risks...)
	}

	slices.SortFunc(gotRisks, func(x, y Risk) int {
		return cmp.Or(
			cmp.Compare(x.Chunk.String(), y.Chunk.String()),
			cmp.Compare(x.Target.String(), y.Target.String()),
		)
	})
	slices.SortFunc(wantRisks, func(x, y Risk) int {
		return cmp.Or(
			cmp.Compare(x.Chunk.String(), y.Chunk.String()),
			cmp.Compare(x.Target.String(), y.Target.String()),
		)
	})

	if !slices.Equal(gotRisks, wantRisks) {
		t.Fatalf(
			"risks = %+v, want exactly the planted forward references %+v",
			gotRisks,
			wantRisks,
		)
	}
}
