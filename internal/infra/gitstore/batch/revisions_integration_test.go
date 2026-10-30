//go:build integration

package batch

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
	"github.com/wrapped-owls/redigit/internal/infra/chunk"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/config"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang"
)

type birthsView struct{ *Revisions }

func (view birthsView) Changes(ctx context.Context) iter.Seq2[indexing.RevisionChange, error] {
	return func(yield func(indexing.RevisionChange, error) bool) {
		for revision, err := range view.Revisions.Changes(ctx) {
			change := indexing.RevisionChange{Rev: revision.Rev}
			for _, one := range revision.Changes {
				change.Files = append(change.Files, indexing.ChangedFile{
					Path: one.Path, Blob: one.Blob, IsDeleted: one.Status == lineage.StatusDeleted,
				})
			}

			if !yield(change, err) {
				return
			}
		}
	}
}

func (view birthsView) ReadBlob(ctx context.Context, oid string) ([]byte, error) {
	out, err := view.Revisions.ReadBlob(ctx, oid)
	if errors.Is(err, lineage.ErrBlobMissing) {
		return nil, fmt.Errorf("%w: %w", indexing.ErrBlobMissing, err)
	}

	return out, err
}

func snapshotOfLog(t testing.TB, revisions *Revisions) entities.Snapshot {
	t.Helper()

	var snap entities.Snapshot

	for revision, err := range revisions.Changes(t.Context()) {
		if err != nil {
			t.Fatalf("Changes() = %v", err)
		}

		snap.History = append(snap.History, entities.SourceTouch{
			Position: entities.Position(len(snap.History)), Rev: revision.Rev,
		})
	}

	return snap
}

func followLineage() lineage.Config { return followed(goLineage()) }

func goLineage() lineage.Config {
	return lineage.Config{
		Concurrency: 8,
		Languages:   []lineage.Language{{Extensions: []string{".go"}, Parser: golang.NewSyntax()}},
	}
}

func TestRevisionsLineageFollowsGitMv(t *testing.T) {
	t.Parallel()

	const (
		alphaSource = "package p\n\nfunc Alpha(a, b int) int {\n\tsum := a + b\n\treturn sum * 2\n}\n\n" +
			"func Beta(a int) int {\n\treturn a - 1\n}\n"
		movedSource = "package p\n\nfunc Gamma(a, b int) int {\n\tsum := a + b\n\treturn sum * 2\n}\n\n" +
			"func Beta(a int) int {\n\treturn a - 1\n}\n\nfunc Delta() int {\n\treturn 7\n}\n"
	)

	repo := newRepo(t)
	commitFiles(t, repo, "one", map[string]string{"a.go": alphaSource})
	gitIn(t, repo, "mv", "a.go", "b.go")
	commitFiles(t, repo, "two", map[string]string{"b.go": movedSource})

	revisions := NewRevisions(repo, "main")

	t.Cleanup(func() { _ = revisions.Close() })

	got, err := lineage.NewScanner(revisions, followLineage()).
		Scan(t.Context(), snapshotOfLog(t, revisions))
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	key := func(file, name string, kind entities.DeclKind) entities.DeclKey {
		return entities.DeclKey{File: file, Kind: kind, Name: name}
	}

	testCases := []struct {
		name     string
		decl     entities.DeclKey
		wantAt   entities.Position
		wantText bool
		wantVia  bool
	}{
		{"kept by name through the move", key("b.go", "Beta", entities.KindFunc), 0, false, true},
		{
			"renamed by text through the move",
			key("b.go", "Gamma", entities.KindFunc), 0, true, true,
		},
		{"added after the move", key("b.go", "Delta", entities.KindFunc), 1, false, false},
		{"the old file keeps its own", key("a.go", "Alpha", entities.KindFunc), 0, false, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			birth, ok := got.Births[testCase.decl]
			if !ok || birth.At != testCase.wantAt || (len(birth.Via) > 0) != testCase.wantVia {
				t.Fatalf("birth of %v = %+v (found %v)", testCase.decl, birth, ok)
			}

			if testCase.wantVia && birth.Via[len(birth.Via)-1].ByText() != testCase.wantText {
				t.Fatalf("via of %v = %+v", testCase.decl, birth.Via)
			}
		})
	}
}

func TestLineageRealData(t *testing.T) {
	repo, ref := realTarget(t)

	revisions := NewRevisions(repo, ref)

	t.Cleanup(func() { _ = revisions.Close() })

	snap := snapshotOfLog(t, revisions)

	startBirths := time.Now()

	births, err := indexing.NewScanner(birthsView{revisions}, prodBirths()).Scan(t.Context(), snap)
	if err != nil {
		t.Fatalf("births = %v", err)
	}

	birthsWall := time.Since(startBirths)

	off, offWall := timedScan(t, revisions, snap, prodLineage())
	on, onWall := timedScan(t, revisions, snap, followed(prodLineage()))
	again, _ := timedScan(t, revisions, snap, followed(prodLineage()))

	assertSameAsBirths(t, off, births)
	assertFollowEarlier(t, on, off)

	if !reflect.DeepEqual(on, again) {
		t.Errorf("two follow runs differ")
	}

	byName, byText := countRenames(on)

	t.Logf("commits=%d decls=%d renamed_by_name=%d renamed_by_text=%d unreadable=%d",
		len(snap.History), len(on.Births), byName, byText, len(on.Unreadable))
	t.Logf("follow_off_equals_births=%v", !t.Failed())
	t.Logf("births wall=%s follow_off wall=%s follow_on wall=%s ratio_off=%.2f ratio_on=%.2f",
		birthsWall, offWall, onWall,
		float64(offWall)/float64(birthsWall), float64(onWall)/float64(birthsWall))
}

func timedScan(
	t *testing.T, revisions *Revisions, snap entities.Snapshot, cfg lineage.Config,
) (lineage.Result, time.Duration) {
	t.Helper()

	start := time.Now()

	result, err := lineage.NewScanner(revisions, cfg).Scan(t.Context(), snap)
	if err != nil {
		t.Fatalf("Scan(follow=%v) = %v", cfg.Follow, err)
	}

	return result, time.Since(start)
}

func assertSameAsBirths(t *testing.T, got lineage.Result, want indexing.Births) {
	t.Helper()

	at := make(map[entities.DeclKey]entities.Position, len(got.Births))
	for key, birth := range got.Births {
		at[key] = birth.At
	}

	if !reflect.DeepEqual(at, want.At) {
		t.Errorf("follow off differs from births: %d vs %d decls", len(at), len(want.At))
	}

	if !slices.Equal(got.Unreadable, want.Unreadable) {
		t.Errorf("unreadable differs: %d vs %d", len(got.Unreadable), len(want.Unreadable))
	}
}

func assertFollowEarlier(t *testing.T, on, off lineage.Result) {
	t.Helper()

	if len(on.Births) != len(off.Births) {
		t.Errorf("follow changed the set of declarations")
	}

	for key, birth := range on.Births {
		base, known := off.Births[key]
		if !known {
			t.Errorf("%v exists only with follow on", key)

			continue
		}

		if birth.At > base.At {
			t.Errorf("%v moved later: %d vs %d", key, birth.At, base.At)
		}

		if (birth.At < base.At) != (len(birth.Via) > 0) {
			t.Errorf("%v: via %v does not explain %d vs %d", key, birth.Via, birth.At, base.At)
		}
	}
}

func countRenames(result lineage.Result) (byName, byText int) {
	for _, birth := range result.Births {
		if len(birth.Via) == 0 {
			continue
		}

		if birth.Via[len(birth.Via)-1].ByText() {
			byText++
		} else {
			byName++
		}
	}

	return byName, byText
}

func prodSyntax() config.Syntax {
	chunkers := make([]config.Chunker, 0, len(chunk.Registry))
	for _, chunker := range chunk.Registry {
		chunkers = append(chunkers, chunker)
	}

	return config.NewSyntax(chunkers)
}

func prodExtensions() []string {
	return []string{
		".mod", ".work", ".sum", ".sql",
		".md", ".yml", ".yaml", ".xml", ".json", ".toml", ".nix", ".lock",
		".env", ".iml", ".mmd", ".gitignore", ".containerignore", "",
	}
}

func prodBirths() indexing.Config {
	return indexing.Config{
		Concurrency: 8,
		Languages: []indexing.Language{
			{Extensions: []string{".go"}, Syntax: golang.NewSyntax()},
			{Extensions: prodExtensions(), Syntax: prodSyntax()},
		},
	}
}

func prodLineage() lineage.Config {
	return lineage.Config{
		Concurrency: 8,
		Languages: []lineage.Language{
			{Extensions: []string{".go"}, Parser: golang.NewSyntax()},
			{Extensions: prodExtensions(), Parser: prodSyntax()},
		},
	}
}

func followed(cfg lineage.Config) lineage.Config {
	cfg.Follow = true

	return cfg
}

func realTarget(t *testing.T) (repo, ref string) {
	t.Helper()

	repo, ref = os.Getenv("REDIGIT_LINEAGE_REPO"), os.Getenv("REDIGIT_LINEAGE_REV")
	if repo == "" && ref == "" {
		t.Skip("REDIGIT_LINEAGE_REPO and REDIGIT_LINEAGE_REV are unset")
	}

	isRev, err := regexp.MatchString(`^[0-9a-f]{7,64}$`, ref)
	if err != nil {
		t.Fatalf("match revision: %v", err)
	}

	info, statErr := os.Stat(repo)
	isRepo := statErr == nil && info.IsDir() && filepath.IsAbs(repo)

	if !isRev || !isRepo {
		t.Fatalf("REDIGIT_LINEAGE_REPO=%q or REDIGIT_LINEAGE_REV=%q is malformed", repo, ref)
	}

	return repo, ref
}
