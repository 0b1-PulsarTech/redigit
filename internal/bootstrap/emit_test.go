package bootstrap

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
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
