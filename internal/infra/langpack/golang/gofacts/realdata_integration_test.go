//go:build integration

package gofacts

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

func TestRealRepoFacts(t *testing.T) {
	repo, rev := os.Getenv("REDIGIT_FACTS_REPO"), os.Getenv("REDIGIT_FACTS_REV")
	if repo == "" || rev == "" {
		t.Skip("set REDIGIT_FACTS_REPO to a checkout at REDIGIT_FACTS_REV to run")
	}

	requireCheckedOut(t, repo, rev)

	ctx := context.Background()

	snap, decls := realSnapshot(t, repo, rev)
	source := typed.NewSource([]string{"integration"})

	defer func() { _ = source.Close(ctx) }()

	loadStart := time.Now()

	if _, err := source.Packages(ctx, snap); err != nil {
		t.Fatalf("Packages() error = %v", err)
	}

	loadMs := time.Since(loadStart).Milliseconds()

	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	buildStart := time.Now()

	index, err := New(source).Index(ctx, snap, decls)
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	buildMs := time.Since(buildStart).Milliseconds()

	runtime.GC()
	runtime.ReadMemStats(&after)

	t.Logf(
		"load ms=%d build ms=%d extra heap MB=%.2f",
		loadMs, buildMs, float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/(1<<20),
	)
	logCounts(t, index, decls)
	pinIndexerCtor(t, index)
	pinCalls(t, index)
}

func pinCalls(t *testing.T, index *Index) {
	t.Helper()

	const (
		dir = "internal/bootstrap/"
		pkg = "github.com/wrapped-owls/redigit/internal/bootstrap."
	)

	births := testkit.DeclKey(dir+"pipeline.go", entities.KindMethod, "Pipeline.Births")
	validate := testkit.DeclKey(dir+"chunkmap.go", entities.KindFunc, "validateChunkMap")
	scanner := testkit.DeclKey(dir+"pipeline.go", entities.KindType, "scanner")

	testCases := []struct {
		name   string
		site   entities.DeclKey
		callee string
		want   facts.Call
	}{
		{
			"an interface call has no callee key and names its interface",
			births, pkg + "scanner.Scan",
			facts.Call{
				Site: births, Callee: pkg + "scanner.Scan",
				Receiver: facts.TypeRef{Key: scanner, Name: pkg + "scanner"},
			},
		},
		{
			"a generic call carries its inferred type arguments",
			validate, "slices.Sorted",
			facts.Call{
				Site: validate, Callee: "slices.Sorted",
				TypeArgs: []facts.TypeRef{{Name: "string"}},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var got []facts.Call

			for _, call := range index.Calls(testCase.site) {
				if call.Callee == testCase.callee {
					got = append(got, call)
				}
			}

			if !reflect.DeepEqual(got, []facts.Call{testCase.want}) {
				t.Fatalf("Calls(%s) to %s = %+v, want %+v",
					testCase.site, testCase.callee, got, testCase.want)
			}
		})
	}
}

func realSnapshot(t *testing.T, repo, rev string) (entities.Snapshot, []entities.Decl) {
	t.Helper()

	reader := gitstore.NewReader(repo, rev)

	defer func() { _ = reader.Close() }()

	snap, err := reader.Snapshot(t.Context())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	goFiles := make([]string, 0, len(snap.Files))

	for _, file := range snap.Files {
		if strings.HasSuffix(file, ".go") {
			goFiles = append(goFiles, file)
		}
	}

	return snap, testkit.DeclareAll(t, repo, goFiles)
}

func logCounts(t *testing.T, index *Index, decls []entities.Decl) {
	t.Helper()

	calls, typeKeyed, interfaceCalls, nonFuncArgs := 0, 0, 0, 0

	for _, found := range index.calls {
		calls += len(found)

		for _, call := range found {
			if call.CalleeKey.Kind == entities.KindType {
				typeKeyed++
			}

			if call.CalleeKey == (entities.DeclKey{}) && call.Receiver.Key != (entities.DeclKey{}) {
				interfaceCalls++
			}

			for _, arg := range call.FuncArgs {
				if arg.Kind != entities.KindFunc && arg.Kind != entities.KindMethod {
					nonFuncArgs++
				}
			}
		}
	}

	t.Logf(
		"calls=%d keyed by a type=%d keyless on a receiver=%d non-func args=%d",
		calls, typeKeyed, interfaceCalls, nonFuncArgs,
	)

	implStart := time.Now()
	pairs, interfaces := 0, 0

	for _, decl := range decls {
		if found := index.Implementors(decl.Key); len(found) > 0 {
			pairs, interfaces = pairs+len(found), interfaces+1
		}
	}

	t.Logf(
		"decls=%d call sites=%d calling decls=%d constructors=%d interfaces=%d implementors=%d",
		len(decls), calls, len(index.calls), len(index.ctors), interfaces, pairs,
	)
	t.Logf("implementors of every decl ms=%d", time.Since(implStart).Milliseconds())
}

func pinIndexerCtor(t *testing.T, index *Index) {
	t.Helper()

	const (
		dir    = "internal/domain/usecases/indexing/"
		prefix = "github.com/wrapped-owls/redigit/internal/domain/usecases/indexing."
	)

	pin := testkit.DeclKey(dir+"indexer.go", entities.KindFunc, "New")
	want := []facts.TypeRef{
		{
			Key:  testkit.DeclKey(dir+"ports.go", entities.KindType, "Resolver"),
			Name: prefix + "Resolver",
		},
		{Key: testkit.DeclKey(dir+"ports.go", entities.KindType, "Tree"), Name: prefix + "Tree"},
		{
			Key:  testkit.DeclKey(dir+"indexer.go", entities.KindType, "Config"),
			Name: prefix + "Config",
		},
	}

	got, err := index.CtorArgTypes(pin)
	if err != nil {
		t.Fatalf("CtorArgTypes(%s) error = %v", pin, err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CtorArgTypes(%s) = %+v, want %+v", pin, got, want)
	}
}

func requireCheckedOut(t *testing.T, repo, rev string) {
	t.Helper()

	heads := make([]string, 0, 2)

	for _, ref := range []string{"HEAD", rev} {
		out, err := exec.CommandContext(
			t.Context(), "git", "-C", repo, "rev-parse", ref+"^{commit}",
		).Output()
		if err != nil {
			t.Fatalf("git rev-parse %s error = %v", ref, err)
		}

		heads = append(heads, strings.TrimSpace(string(out)))
	}

	if heads[0] != heads[1] {
		t.Fatalf("%s is checked out at %s, want %s", repo, heads[0], heads[1])
	}
}
