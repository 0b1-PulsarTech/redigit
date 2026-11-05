package bootstrap

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
	"github.com/wrapped-owls/redigit/internal/infra/lsp"
)

func birthKey(file, name string) entities.DeclKey {
	return entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name}
}

func TestCapabilitiesFrom(t *testing.T) {
	t.Parallel()

	early := birthKey("lead/repo.go", "Insert")
	late := birthKey("lead/repo.go", "Archive")
	orphan := birthKey("tools/gen.go", "Main")

	births := indexing.Births{At: map[entities.DeclKey]entities.Position{
		early:  5,
		late:   300,
		orphan: 900,
	}}

	features := entities.NewFeatures([]entities.Feature{
		{Name: "captacao", At: []entities.Position{5, 6, 7}},
		{Name: "arquivamento", At: []entities.Position{300}},
	})

	got := capabilitiesFrom(births, features)

	testCases := []struct {
		name string
		key  entities.DeclKey
		want planning.FeatureID
	}{
		{
			// The two declarations share a FILE and belong to different
			// slices: dating them by the file would file both under the
			// first, which is the whole reason the birth is per declaration.
			name: "an early declaration of a file",
			key:  early,
			want: "captacao",
		},
		{
			name: "a declaration the same file grew later",
			key:  late,
			want: "arquivamento",
		},
		{
			name: "a declaration born where no slice reaches",
			key:  orphan,
			want: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got[testCase.key] != testCase.want {
				t.Fatalf("capability = %q, want %q", got[testCase.key], testCase.want)
			}
		})
	}
}

func TestCapabilitiesFromWithNoFeaturesIsNil(t *testing.T) {
	t.Parallel()

	births := indexing.Births{At: map[entities.DeclKey]entities.Position{
		birthKey("a.go", "A"): 1,
	}}

	if got := capabilitiesFrom(births, entities.Features{}); got != nil {
		t.Fatalf("capabilitiesFrom() = %v, want nil so the planner derives its own", got)
	}
}

func TestWireResolver(t *testing.T) {
	t.Parallel()

	errStart := errors.New("no server")

	testCases := []struct {
		name       string
		resolver   string
		wantStarts int32
		wantErr    error
	}{
		{"lsp starts the server at once", ResolverLSP, 1, errStart},
		{"the zero value is typed", "", 0, nil},
		{"typed starts nothing", ResolverTyped, 0, nil},
		{"an unknown name is refused", "gopls", 0, ErrResolver},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var starts atomic.Int32

			wired, err := wireResolver(
				context.Background(),
				Config{Resolver: testCase.resolver},
				entities.Snapshot{},
				typed.NewSource(nil),
				func(context.Context) (lsp.Backend, error) {
					starts.Add(1)

					return nil, errStart
				},
			)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("wireResolver() error = %v, want %v", err, testCase.wantErr)
			}

			if starts.Load() != testCase.wantStarts {
				t.Fatalf("starts = %d, want %d", starts.Load(), testCase.wantStarts)
			}

			if testCase.wantErr == nil && (wired.resolver == nil || wired.session == nil) {
				t.Fatalf("wireResolver() = %+v, want a resolver and a session", wired)
			}
		})
	}
}

func TestCheckResolver(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"lsp", ResolverLSP, nil},
		{"typed", ResolverTyped, nil},
		{"unknown", "gopls", ErrResolver},
		{"empty", "", ErrResolver},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := CheckResolver(testCase.input); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("CheckResolver(%q) = %v, want %v", testCase.input, err, testCase.wantErr)
			}
		})
	}
}

type askingIndexer struct {
	resolver indexing.Resolver
	queries  int
	err      error
}

func (asking askingIndexer) Index(context.Context, entities.Snapshot) (indexing.Result, error) {
	for range asking.queries {
		_, _ = asking.resolver.Definition(context.Background(), entities.Location{File: "a.go"})
	}

	return indexing.Result{}, asking.err
}

type quietBackend struct{ calls *atomic.Int32 }

func (backend quietBackend) References(
	context.Context, entities.Location,
) ([]entities.Location, error) {
	backend.calls.Add(1)

	return nil, nil
}

func (backend quietBackend) Implementations(
	context.Context, entities.Location,
) ([]entities.Location, error) {
	backend.calls.Add(1)

	return nil, nil
}

func (backend quietBackend) Definition(
	context.Context,
	entities.Location,
) (entities.Location, error) {
	backend.calls.Add(1)

	return entities.Location{}, nil
}

func (backend quietBackend) Close(context.Context) error { return nil }

func TestGuardedIndexRefusesAFailedStart(t *testing.T) {
	t.Parallel()

	errServer := errors.New("no server")
	errIndex := errors.New("index")

	testCases := []struct {
		name       string
		resolver   string
		startErr   error
		queries    int
		indexErr   error
		wantStarts int32
		wantErr    error
		wantAlso   error
	}{
		{"a failed start refuses the run", ResolverTyped, errServer, 3, nil, 1, lsp.ErrStart, nil},
		{
			"the failure outranks an index error without dropping it",
			ResolverTyped, errServer, 1, errIndex, 1, lsp.ErrStart, errIndex,
		},
		{"the first fallback starts the server once", ResolverTyped, nil, 3, nil, 1, nil, nil},
		{"no fallback starts nothing", ResolverTyped, errServer, 0, nil, 0, nil, nil},
		{
			"an index error alone passes through",
			ResolverTyped, nil, 0, errIndex, 0, errIndex, nil,
		},
		{"lsp mode is not guarded", ResolverLSP, nil, 2, nil, 1, nil, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var starts, calls atomic.Int32

			wired, err := wireResolver(
				context.Background(),
				Config{Resolver: testCase.resolver},
				entities.Snapshot{},
				typed.NewSource(nil),
				func(context.Context) (lsp.Backend, error) {
					starts.Add(1)

					if testCase.startErr != nil {
						return nil, testCase.startErr
					}

					return quietBackend{calls: &calls}, nil
				},
			)
			if err != nil {
				t.Fatalf("wireResolver() error = %v", err)
			}

			_, err = wired.guard(askingIndexer{
				resolver: wired.resolver, queries: testCase.queries, err: testCase.indexErr,
			}).Index(context.Background(), entities.Snapshot{})
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Index() error = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantAlso != nil && !errors.Is(err, testCase.wantAlso) {
				t.Fatalf("Index() error = %v, want it to keep %v", err, testCase.wantAlso)
			}

			if starts.Load() != testCase.wantStarts {
				t.Fatalf("starts = %d, want %d", starts.Load(), testCase.wantStarts)
			}

			if testCase.startErr != nil && calls.Load() != 0 {
				t.Fatalf("backend calls = %d, want 0 after a failed start", calls.Load())
			}

			if closeErr := wired.session.Close(context.Background()); closeErr != nil {
				t.Fatalf("Close() error = %v, want a no-op after a failed start", closeErr)
			}
		})
	}
}

func TestBuildFlags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		tags []string
		want []string
	}{
		{"no tags yields nil, not -tags=", nil, nil},
		{"empty slice also yields nil", []string{}, nil},
		{"one tag", []string{"integration"}, []string{"-tags=integration"}},
		{
			"multiple tags share one flag, as go keeps only the last",
			[]string{"integration", "e2e"},
			[]string{"-tags=integration,e2e"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := buildFlags(testCase.tags)

			if testCase.want == nil {
				if got != nil {
					t.Fatalf("buildFlags(%v) = %v, want nil", testCase.tags, got)
				}

				return
			}

			if len(got) != len(testCase.want) {
				t.Fatalf("buildFlags(%v) = %v, want %v", testCase.tags, got, testCase.want)
			}

			for i, flag := range testCase.want {
				if got[i] != flag {
					t.Fatalf("buildFlags(%v)[%d] = %q, want %q", testCase.tags, i, got[i], flag)
				}
			}
		})
	}
}
