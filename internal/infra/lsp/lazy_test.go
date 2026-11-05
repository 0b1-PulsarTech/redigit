package lsp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var errStart = errors.New("no server")

func TestLazySessionStartsOnFirstQuery(t *testing.T) {
	t.Parallel()

	at := entities.Location{File: "a.go", Line: 1, Char: 2}
	want := []entities.Location{{File: "b.go"}}

	testCases := []struct {
		name  string
		query func(*LazySession) error
		setup func(*MockBackend)
	}{
		{
			"references",
			func(lazy *LazySession) error {
				_, err := lazy.References(context.Background(), at)

				return err
			},
			func(backend *MockBackend) { backend.EXPECT().References(gomock.Any(), at).Return(want, nil) },
		},
		{
			"implementations",
			func(lazy *LazySession) error {
				_, err := lazy.Implementations(context.Background(), at)

				return err
			},
			func(backend *MockBackend) {
				backend.EXPECT().Implementations(gomock.Any(), at).Return(want, nil)
			},
		},
		{
			"definition",
			func(lazy *LazySession) error {
				_, err := lazy.Definition(context.Background(), at)

				return err
			},
			func(backend *MockBackend) {
				backend.EXPECT().Definition(gomock.Any(), at).Return(want[0], nil)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			backend := NewMockBackend(gomock.NewController(t))
			testCase.setup(backend)

			var started atomic.Int32

			lazy := NewLazySession(func(context.Context) (Backend, error) {
				started.Add(1)

				return backend, nil
			})

			if started.Load() != 0 {
				t.Fatal("the server started before the first query")
			}

			if err := testCase.query(lazy); err != nil {
				t.Fatalf("query error = %v", err)
			}

			if started.Load() != 1 {
				t.Fatalf("starts = %d, want 1", started.Load())
			}
		})
	}
}

func TestLazySessionClose(t *testing.T) {
	t.Parallel()

	t.Run("never started is a no-op", func(t *testing.T) {
		t.Parallel()

		lazy := NewLazySession(func(context.Context) (Backend, error) {
			t.Error("start called")

			return nil, errStart
		})

		if err := lazy.Close(context.Background()); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	t.Run("started closes the backend", func(t *testing.T) {
		t.Parallel()

		backend := NewMockBackend(gomock.NewController(t))
		backend.EXPECT().Definition(gomock.Any(), gomock.Any()).Return(entities.Location{}, nil)
		backend.EXPECT().Close(gomock.Any()).Return(errStart)

		lazy := NewLazySession(func(context.Context) (Backend, error) { return backend, nil })

		if _, err := lazy.Definition(context.Background(), entities.Location{}); err != nil {
			t.Fatalf("Definition() error = %v", err)
		}

		if err := lazy.Close(context.Background()); !errors.Is(err, errStart) {
			t.Fatalf("Close() error = %v, want %v", err, errStart)
		}
	})
}

func TestLazySessionQueryAfterClose(t *testing.T) {
	t.Parallel()

	at := entities.Location{File: "a.go"}

	testCases := []struct {
		name  string
		query func(*LazySession) error
	}{
		{"references", func(lazy *LazySession) error {
			_, err := lazy.References(context.Background(), at)

			return err
		}},
		{"implementations", func(lazy *LazySession) error {
			_, err := lazy.Implementations(context.Background(), at)

			return err
		}},
		{"definition", func(lazy *LazySession) error {
			_, err := lazy.Definition(context.Background(), at)

			return err
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			lazy := NewLazySession(func(context.Context) (Backend, error) {
				t.Error("start called after Close")

				return nil, errStart
			})

			if err := lazy.Close(context.Background()); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			if err := testCase.query(lazy); !errors.Is(err, ErrClosed) {
				t.Fatalf("query error = %v, want %v", err, ErrClosed)
			}
		})
	}
}

func TestLazySessionStartsOnceUnderContention(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		backend bool
		wantErr error
	}{
		{"a working server", true, nil},
		{"a server that does not start", false, errStart},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				started atomic.Int32
				backend Backend
			)

			if testCase.backend {
				mock := NewMockBackend(gomock.NewController(t))
				mock.EXPECT().Definition(gomock.Any(), gomock.Any()).
					Return(entities.Location{}, nil).Times(8)

				backend = mock
			}

			lazy := NewLazySession(func(context.Context) (Backend, error) {
				started.Add(1)

				if backend == nil {
					return nil, errStart
				}

				return backend, nil
			})

			var (
				group sync.WaitGroup
				begin = make(chan struct{})
				errs  = make([]error, 8)
			)

			for slot := range errs {
				group.Go(func() {
					<-begin

					_, errs[slot] = lazy.Definition(context.Background(), entities.Location{})
				})
			}

			close(begin)
			group.Wait()

			if started.Load() != 1 {
				t.Fatalf("starts = %d, want 1", started.Load())
			}

			for slot, err := range errs {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("caller %d error = %v, want %v", slot, err, testCase.wantErr)
				}
			}
		})
	}
}

func TestLazySessionFailedStart(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		query func(*LazySession) error
	}{
		{
			"a references query",
			func(lazy *LazySession) error {
				_, err := lazy.References(context.Background(), entities.Location{})

				return err
			},
		},
		{
			"an implementations query",
			func(lazy *LazySession) error {
				_, err := lazy.Implementations(context.Background(), entities.Location{})

				return err
			},
		},
		{
			"a definition query",
			func(lazy *LazySession) error {
				_, err := lazy.Definition(context.Background(), entities.Location{})

				return err
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			untouched := NewMockBackend(gomock.NewController(t))

			var started atomic.Int32

			lazy := NewLazySession(func(context.Context) (Backend, error) {
				started.Add(1)

				return untouched, errStart
			})

			err := testCase.query(lazy)
			if !errors.Is(err, ErrStart) || !errors.Is(err, errStart) {
				t.Fatalf("query error = %v, want ErrStart wrapping the start error", err)
			}

			if again := testCase.query(lazy); !errors.Is(again, ErrStart) || started.Load() != 1 {
				t.Fatalf(
					"second query error = %v after %d starts, want ErrStart and 1",
					again,
					started.Load(),
				)
			}

			if closeErr := lazy.Close(context.Background()); closeErr != nil {
				t.Fatalf("Close() error = %v, want a no-op after a failed start", closeErr)
			}
		})
	}
}

func TestServerConfig_mergedInitOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		cfg            ServerConfig
		wantBuildFlags []string
		wantExtraKey   string
	}{
		{
			name:           "build flags only",
			cfg:            ServerConfig{BuildFlags: []string{"-tags=integration"}},
			wantBuildFlags: []string{"-tags=integration"},
		},
		{
			name:           "nil build flags is explicit, not omitted",
			cfg:            ServerConfig{},
			wantBuildFlags: nil,
		},
		{
			name: "extra options merged alongside build flags",
			cfg: ServerConfig{
				BuildFlags: []string{"-tags=integration"},
				InitializationOptions: map[string]any{
					"env": map[string]string{"GOFLAGS": "-mod=mod"},
				},
			},
			wantBuildFlags: []string{"-tags=integration"},
			wantExtraKey:   "env",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := testCase.cfg.mergedInitOptions()

			flags, ok := got["buildFlags"]
			if !ok {
				t.Fatalf("mergedInitOptions: missing buildFlags key entirely: %v", got)
			}

			gotFlags, _ := flags.([]string)
			if len(gotFlags) != len(testCase.wantBuildFlags) {
				t.Fatalf("buildFlags = %v, want %v", gotFlags, testCase.wantBuildFlags)
			}

			if testCase.wantExtraKey != "" {
				if _, ok := got[testCase.wantExtraKey]; !ok {
					t.Fatalf(
						"mergedInitOptions: missing extra key %q: %v",
						testCase.wantExtraKey,
						got,
					)
				}
			}
		})
	}
}
