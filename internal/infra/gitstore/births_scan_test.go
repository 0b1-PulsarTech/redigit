package gitstore

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang"
)

func TestBirthsScanOnRepo(t *testing.T) {
	repo := os.Getenv("REDIGIT_BENCH_REPO")
	if repo == "" {
		t.Skip("REDIGIT_BENCH_REPO is unset")
	}

	ref := "HEAD"
	if value := os.Getenv("REDIGIT_BENCH_REF"); value != "" {
		ref = value
	}

	reader := NewReader(repo, ref)
	if closer, ok := any(reader).(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}

	startSnapshot := time.Now()

	snap, err := reader.Snapshot(t.Context())
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}

	snapshotWall := time.Since(startSnapshot)

	scanner := indexing.NewScanner(reader, indexing.Config{
		Concurrency: 8,
		Languages: []indexing.Language{
			{Name: "go", Extensions: []string{".go"}, Syntax: golang.NewSyntax()},
		},
	})

	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	start := time.Now()

	births, err := scanner.Scan(t.Context(), snap)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	wall := time.Since(start)

	runtime.ReadMemStats(&after)

	t.Logf("snapshot wall=%s commits=%d", snapshotWall, len(snap.History))
	t.Logf("scan wall=%s allocs=%d alloc_bytes=%d births=%d unreadable=%d",
		wall, after.Mallocs-before.Mallocs, after.TotalAlloc-before.TotalAlloc,
		len(births.At), len(births.Unreadable))

	dump := os.Getenv("REDIGIT_BENCH_DUMP")
	if dump == "" {
		return
	}

	lines := make([]string, 0, len(births.At)+len(births.Unreadable))
	for key, position := range births.At {
		lines = append(lines, fmt.Sprintf("B %v %d", key, position))
	}

	for _, entry := range births.Unreadable {
		lines = append(lines, "U "+entry)
	}

	slices.Sort(lines)

	if err = os.WriteFile(dump, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write dump: %v", err)
	}
}
