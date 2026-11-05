package bootstrap

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestClosersCloseInReverseAndJoinEveryError(t *testing.T) {
	t.Parallel()

	errFirst := errors.New("first would not close")
	errLast := errors.New("last would not close")

	testCases := []struct {
		name      string
		build     func(order *[]string) closers
		wantOrder []string
		wantErrs  []error
	}{
		{name: "none held", build: func(*[]string) closers { return nil }},
		{
			name: "closed last to first",
			build: func(order *[]string) closers {
				return closers{orderedCloser{"a", order}, orderedCloser{"b", order}}
			},
			wantOrder: []string{"b", "a"},
		},
		{
			name: "one failure does not stop the rest",
			build: func(order *[]string) closers {
				return closers{
					failingCloser{orderedCloser{"a", order}, errFirst},
					orderedCloser{"b", order},
					failingCloser{orderedCloser{"c", order}, errLast},
				}
			},
			wantOrder: []string{"c", "b", "a"},
			wantErrs:  []error{errFirst, errLast},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var order []string

			err := testCase.build(&order).Close(t.Context())

			if !slices.Equal(order, testCase.wantOrder) {
				t.Fatalf("Close() stopped %v, want %v", order, testCase.wantOrder)
			}

			for _, target := range testCase.wantErrs {
				if !errors.Is(err, target) {
					t.Fatalf("Close() = %v, want %v wrapped", err, target)
				}
			}

			if len(testCase.wantErrs) == 0 && err != nil {
				t.Fatalf("Close() = %v, want nil", err)
			}
		})
	}
}

type admittingStub struct {
	stubRenderer

	failure error
}

func (stub admittingStub) Admit(
	_ context.Context,
	source []byte,
	specs []string,
) ([]byte, error) {
	return []byte(stub.tag + ":" + string(source) + ":" + strings.Join(specs, ",")), stub.failure
}

func TestByExtensionAdmitsThroughTheLanguageOfTheFile(t *testing.T) {
	t.Parallel()

	failure := errors.New("clash")
	dispatch := newRenderer(
		stubRenderer{tag: "fallback"},
		map[string]renderer{
			".go":  admittingStub{stubRenderer: stubRenderer{tag: "go"}},
			".bad": admittingStub{stubRenderer: stubRenderer{tag: "bad"}, failure: failure},
			".mod": stubRenderer{tag: "config"},
		},
	)

	testCases := []struct {
		name    string
		file    string
		want    string
		wantErr error
	}{
		{name: "a Go file admits through the Go syntax", file: "a/main.go", want: "go:src:x,y"},
		{name: "a language that cannot admit leaves the source", file: "go.mod", want: "src"},
		{name: "an extension nothing claims leaves the source", file: "README.md", want: "src"},
		{name: "a refused admission names the extension", file: "a.bad", wantErr: failure},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := dispatch.Admit(
				t.Context(),
				testCase.file,
				[]byte("src"),
				[]string{"x", "y"},
			)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Admit() error = %v, want %v", err, testCase.wantErr)
			}

			if err == nil && string(got) != testCase.want {
				t.Fatalf("Admit() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestPipelineCloseFoldsBothFailures pins the defect this task exists to
// prevent from recurring silently: Close used to stop at the first failure,
// so a checkout left behind by a broken session close was never reported.
func TestPipelineCloseFoldsBothFailures(t *testing.T) {
	t.Parallel()

	errSession := errors.New("language server would not stop")
	errReader := errors.New("cat-file would not stop")
	errCheckout := errors.New("checkout is still locked")
	errLoads := errors.New("typed load would not stop")

	testCases := []struct {
		name     string
		loads    fakeCloser
		session  fakeCloser
		reader   fakeCloser
		checkout fakeCloser
		want     []error
	}{
		{name: "all close cleanly"},
		{
			name:  "only the typed loads fail",
			loads: fakeCloser{err: errLoads},
			want:  []error{errLoads},
		},
		{
			name:    "only the session fails",
			session: fakeCloser{err: errSession},
			want:    []error{errSession},
		},
		{
			name:     "only the checkout fails",
			checkout: fakeCloser{err: errCheckout},
			want:     []error{errCheckout},
		},
		{
			name:     "the session fails, so the checkout it may still read is left alone",
			session:  fakeCloser{err: errSession},
			checkout: fakeCloser{err: errCheckout},
			want:     []error{errSession},
		},
		{
			name:   "only the git reader fails",
			reader: fakeCloser{err: errReader},
			want:   []error{errReader},
		},
		{
			name:     "the reader fails beside a session that keeps the checkout",
			session:  fakeCloser{err: errSession},
			reader:   fakeCloser{err: errReader},
			checkout: fakeCloser{err: errCheckout},
			want:     []error{errSession, errReader},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pipeline := &Pipeline{
				loads:    testCase.loads,
				session:  testCase.session,
				reader:   testCase.reader,
				checkout: testCase.checkout,
			}

			err := pipeline.Close(t.Context())
			if len(testCase.want) == 0 && err != nil {
				t.Fatalf("Close() = %v, want nil", err)
			}

			for _, target := range []error{errLoads, errSession, errReader, errCheckout} {
				if errors.Is(err, target) != slices.Contains(testCase.want, target) {
					t.Fatalf("Close() = %v, want exactly %v wrapped", err, testCase.want)
				}
			}
		})
	}
}

type orderedCloser struct {
	name  string
	order *[]string
}

func (closing orderedCloser) Close(context.Context) error {
	*closing.order = append(*closing.order, closing.name)

	return nil
}

func TestPipelineCloseStopsTheResolverBeforeTheLoadsAndTheCheckoutGoes(t *testing.T) {
	t.Parallel()

	var order []string

	pipeline := &Pipeline{
		loads:    orderedCloser{"loads", &order},
		session:  orderedCloser{"session", &order},
		reader:   orderedCloser{"reader", &order},
		checkout: orderedCloser{"checkout", &order},
	}

	if err := pipeline.Close(t.Context()); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	want := []string{"session", "loads", "reader", "checkout"}
	if !slices.Equal(order, want) {
		t.Fatalf("Close() stopped %v, want %v", order, want)
	}
}

func TestPipelineCloseKeepsTheCheckoutWhenLoadsStay(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		loadsErr   error
		sessionErr error
		wantOrder  []string
		wantKept   bool
	}{
		{"the loads stop", nil, nil, []string{"session", "loads", "reader", "checkout"}, false},
		{
			"the close context expired",
			context.DeadlineExceeded,
			nil,
			[]string{"session", "loads", "reader"},
			true,
		},
		{
			"the session did not close",
			nil,
			context.DeadlineExceeded,
			[]string{"session", "loads", "reader"},
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var order []string

			pipeline := &Pipeline{
				loads:    failingCloser{orderedCloser{"loads", &order}, testCase.loadsErr},
				session:  failingCloser{orderedCloser{"session", &order}, testCase.sessionErr},
				reader:   orderedCloser{"reader", &order},
				checkout: orderedCloser{"checkout", &order},
				snapshot: entities.Snapshot{Root: "/tmp/kept-checkout"},
			}

			err := pipeline.Close(t.Context())

			if !slices.Equal(order, testCase.wantOrder) {
				t.Fatalf("Close() stopped %v, want %v", order, testCase.wantOrder)
			}

			if testCase.wantKept {
				if !errors.Is(err, cmp.Or(testCase.loadsErr, testCase.sessionErr)) ||
					!strings.Contains(err.Error(), "/tmp/kept-checkout") {
					t.Fatalf("Close() = %v, want the kept checkout path and the close failure", err)
				}
			} else if err != nil {
				t.Fatalf("Close() = %v, want nil", err)
			}
		})
	}
}

type failingCloser struct {
	orderedCloser

	err error
}

func (closing failingCloser) Close(ctx context.Context) error {
	_ = closing.orderedCloser.Close(ctx)

	return closing.err
}

func TestPipelineCloseStopsTheHookResourcesFirst(t *testing.T) {
	t.Parallel()

	errHook := errors.New("hook process would not stop")

	testCases := []struct {
		name      string
		hookErr   error
		wantOrder []string
	}{
		{
			name:      "before the session and the checkout they may read",
			wantOrder: []string{"hook", "session", "loads", "reader", "checkout"},
		},
		{
			name:      "a hook closer's error surfaces",
			hookErr:   errHook,
			wantOrder: []string{"hook", "session", "loads", "reader", "checkout"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var order []string

			pipeline := &Pipeline{
				hooked:   closers{failingCloser{orderedCloser{"hook", &order}, testCase.hookErr}},
				loads:    orderedCloser{"loads", &order},
				session:  orderedCloser{"session", &order},
				reader:   orderedCloser{"reader", &order},
				checkout: orderedCloser{"checkout", &order},
			}

			err := pipeline.Close(t.Context())

			if !errors.Is(err, testCase.hookErr) {
				t.Fatalf("Close() = %v, want %v", err, testCase.hookErr)
			}

			if !slices.Equal(order, testCase.wantOrder) {
				t.Fatalf("Close() stopped %v, want %v", order, testCase.wantOrder)
			}
		})
	}
}
