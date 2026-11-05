package syntax

import (
	"context"
	"testing"
)

func TestSyntaxDeclarationsIsTest(t *testing.T) {
	t.Parallel()

	const src = `package sample

import "testing"

func TestFoo(t *testing.T) {}
func Testify()              {}
func BenchmarkFoo(b *testing.B) {}
func helper()               {}
`

	decls, err := Declarations(context.Background(), "sample_test.go", []byte(src))
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	got := map[string]bool{}
	for _, decl := range decls {
		got[decl.Key.Name] = decl.IsTest
	}

	testCases := []struct {
		name     string
		declName string
		want     bool
	}{
		{"TestFoo is a test", "TestFoo", true},
		{"Testify is not (lowercase after prefix)", "Testify", false},
		{"BenchmarkFoo is a test", "BenchmarkFoo", true},
		{"helper is not a test", "helper", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got[testCase.declName] != testCase.want {
				t.Fatalf(
					"IsTest[%s] = %v, want %v",
					testCase.declName,
					got[testCase.declName],
					testCase.want,
				)
			}
		})
	}
}

func TestReceiverName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		src  string
		want string
	}{
		{"pointer receiver", "package s\nfunc (r *Repo) M() {}\n", "Repo"},
		{"value receiver", "package s\nfunc (r Repo) M() {}\n", "Repo"},
		{"generic receiver", "package s\nfunc (r *Repo[T]) M() {}\n", "Repo"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decls, err := Declarations(context.Background(), "s.go", []byte(testCase.src))
			if err != nil {
				t.Fatalf("Declarations() error = %v", err)
			}

			want := testCase.want + ".M"
			if decls[0].Key.Name != want {
				t.Fatalf("method name = %q, want %q", decls[0].Key.Name, want)
			}
		})
	}
}
