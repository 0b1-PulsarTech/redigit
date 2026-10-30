package loadonce

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type loadFunc = func(context.Context) (int, error)

func abandoned(once *Once[int], load loadFunc, entered <-chan struct{}) error {
	leaving, leave := context.WithCancel(context.Background())
	left := make(chan error, 1)

	go func() {
		_, err := once.Do(leaving, load)
		left <- err
	}()

	<-entered
	leave()

	return <-left
}

func matches(err, want error) bool {
	if want == nil {
		return err == nil
	}

	return errors.Is(err, want)
}

func TestOnceDo(t *testing.T) {
	t.Parallel()

	errLoad := errors.New("load failed")

	testCases := []struct {
		name      string
		cancelled bool
		panics    bool
		loadErr   error
		wantValue int
		wantErr   error
		wantRan   bool
		wantText  string
	}{
		{"a live caller gets the value", false, false, nil, 7, nil, true, ""},
		{"a failed load has no value", false, false, errLoad, 0, errLoad, true, ""},
		{"a panicking load is an error", false, true, nil, 0, ErrPanicked, true, "TestOnceDo"},
		{"a cancelled caller starts nothing", true, false, nil, 0, context.Canceled, false, ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				once Once[int]
				ran  atomic.Bool
			)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if testCase.cancelled {
				cancel()
			}

			got, err := once.Do(ctx, func(context.Context) (int, error) {
				ran.Store(true)

				if testCase.panics {
					panic("load blew up")
				}

				return 7, testCase.loadErr
			})

			if closeErr := once.Close(context.Background()); closeErr != nil {
				t.Fatalf("Close() error = %v", closeErr)
			}

			if !matches(err, testCase.wantErr) {
				t.Fatalf("Do() error = %v, want %v", err, testCase.wantErr)
			}

			if testCase.wantText != "" && !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("Do() error = %v, want it to name %q", err, testCase.wantText)
			}

			if got != testCase.wantValue {
				t.Fatalf("Do() = %d, want %d", got, testCase.wantValue)
			}

			if ran.Load() != testCase.wantRan {
				t.Fatalf("load ran = %v, want %v", ran.Load(), testCase.wantRan)
			}
		})
	}
}

func TestOnceFinishedLoadOutlivesContext(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		iterations int
		closeFirst bool
	}{
		{"Do after the load finished", 100, false},
		{"Close after the load finished", 100, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for range testCase.iterations {
				var once Once[int]

				if got, err := once.Do(context.Background(), func(context.Context) (int, error) {
					return 7, nil
				}); err != nil || got != 7 {
					t.Fatalf("first Do() = %d, %v, want 7, nil", got, err)
				}

				gone, cancel := context.WithCancel(context.Background())
				cancel()

				if testCase.closeFirst {
					if err := once.Close(gone); err != nil {
						t.Fatalf("Close() error = %v, want nil", err)
					}

					continue
				}

				got, err := once.Do(gone, func(context.Context) (int, error) { return 9, nil })
				if err != nil || got != 7 {
					t.Fatalf("Do() = %d, %v, want the cached 7, nil", got, err)
				}
			}
		})
	}
}

func TestOnceSharesOneLoad(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		callers     int
		firstLeaves bool
	}{
		{"callers contend for one load", 16, false},
		{"the first caller leaves mid-load", 4, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				once    Once[int]
				calls   atomic.Int32
				entered = make(chan struct{})
				release = make(chan struct{})
				start   = make(chan struct{})
				group   sync.WaitGroup
			)

			load := func(loadCtx context.Context) (int, error) {
				if calls.Add(1) == 1 {
					close(entered)
				}

				<-release

				return 7, loadCtx.Err()
			}

			if testCase.firstLeaves {
				if err := abandoned(&once, load, entered); !errors.Is(err, context.Canceled) {
					t.Fatalf("Do() by the leaving caller error = %v, want context.Canceled", err)
				}
			}

			failures := make(chan string, testCase.callers)

			for range testCase.callers {
				group.Go(func() {
					<-start

					if got, err := once.Do(context.Background(), load); err != nil || got != 7 {
						failures <- "a caller did not get the shared answer"
					}
				})
			}

			close(start)
			close(release)
			group.Wait()
			close(failures)

			for failure := range failures {
				t.Error(failure)
			}

			if calls.Load() != 1 {
				t.Fatalf("load ran %d times, want once", calls.Load())
			}
		})
	}
}

func TestOnceClose(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		started      bool
		stubborn     bool
		closeGivesUp bool
	}{
		{"nothing started", false, false, false},
		{"a left load is cancelled and waited for", true, false, false},
		{"a stubborn load is waited for only as asked", true, true, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				once     Once[int]
				observed atomic.Bool
				entered  = make(chan struct{})
				release  = make(chan struct{})
			)

			load := blocking(testCase.stubborn, entered, release, &observed)

			if testCase.started {
				leaveLoadRunning(t, &once, load, entered, &observed)
			}

			closeCtx, giveUp := context.WithCancel(context.Background())
			defer giveUp()

			var wantCloseErr error

			if testCase.closeGivesUp {
				giveUp()

				wantCloseErr = context.Canceled
			}

			if closeErr := once.Close(closeCtx); !matches(closeErr, wantCloseErr) {
				t.Fatalf("Close() error = %v, want %v", closeErr, wantCloseErr)
			}

			if want := testCase.started && !testCase.stubborn; observed.Load() != want {
				t.Fatalf("load observed cancellation = %v, want %v", observed.Load(), want)
			}

			if testCase.stubborn {
				close(release)
			}

			wantDoErr := context.Canceled
			if !testCase.started {
				wantDoErr = ErrClosed
			}

			assertClosed(t, &once, wantDoErr)
		})
	}
}

func leaveLoadRunning(
	t *testing.T,
	once *Once[int],
	load loadFunc,
	entered <-chan struct{},
	observed *atomic.Bool,
) {
	t.Helper()

	if err := abandoned(once, load, entered); !errors.Is(err, context.Canceled) {
		t.Fatalf("Do() by the leaving caller error = %v, want context.Canceled", err)
	}

	if observed.Load() {
		t.Fatal("a caller leaving cancelled the load other callers may share")
	}
}

func assertClosed(t *testing.T, once *Once[int], wantDoErr error) {
	t.Helper()

	if err := once.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	_, doErr := once.Do(context.Background(), func(context.Context) (int, error) {
		return 1, nil
	})
	if !errors.Is(doErr, wantDoErr) {
		t.Fatalf("Do() after Close error = %v, want %v", doErr, wantDoErr)
	}
}

func blocking(
	stubborn bool,
	entered chan<- struct{},
	release <-chan struct{},
	observed *atomic.Bool,
) loadFunc {
	return func(loadCtx context.Context) (int, error) {
		close(entered)

		if stubborn {
			<-release
		} else {
			<-loadCtx.Done()
			observed.Store(true)
		}

		return 0, loadCtx.Err()
	}
}
