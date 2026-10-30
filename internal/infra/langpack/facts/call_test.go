package facts

import (
	"errors"
	"path"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestTypeRefIsZero(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		ref  TypeRef
		want bool
	}{
		{"nothing set", TypeRef{}, true},
		{"a name only", TypeRef{Name: "net/http.Client"}, false},
		{"a key only", TypeRef{Key: entities.DeclKey{File: "a.go"}}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.ref.IsZero(); got != testCase.want {
				t.Fatalf("IsZero() = %t, want %t", got, testCase.want)
			}
		})
	}
}

func TestCallShort(t *testing.T) {
	t.Parallel()

	receiver := TypeRef{Name: "example.test/fixture/p.Repo"}

	testCases := []struct {
		name string
		call Call
		want string
	}{
		{"a function", Call{Callee: "example.test/fixture/p.New"}, "New"},
		{
			"a method",
			Call{Callee: "example.test/fixture/p.Repo.Get", Receiver: receiver},
			"Repo.Get",
		},
		{
			"a function of a path with a dotted element",
			Call{Callee: "gopkg.in/yaml.v3.Marshal"},
			"Marshal",
		},
		{
			"a method of a path with a dotted element",
			Call{
				Callee:   "gopkg.in/yaml.v3.Node.Decode",
				Receiver: TypeRef{Name: "gopkg.in/yaml.v3.Node"},
			},
			"Node.Decode",
		},
		{"a name with no package", Call{Callee: "New"}, "New"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.call.Short(); got != testCase.want {
				t.Fatalf("Short() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestCallMatches(t *testing.T) {
	t.Parallel()

	function := Call{Callee: "example.test/fixture/p.New"}
	method := Call{
		Callee:   "example.test/fixture/p.Repo.Get",
		Receiver: TypeRef{Name: "example.test/fixture/p.Repo"},
	}

	testCases := []struct {
		name    string
		call    Call
		pattern string
		want    bool
		wantErr error
	}{
		{"a function by its short name", function, "New", true, nil},
		{"a method by its short name", method, "Repo.Get", true, nil},
		{"a method by a wildcard method", method, "Repo.*", true, nil},
		{"a method by the type alone", method, "Get", false, nil},
		{"a call by its qualified name", function, "example.test/fixture/p.New", true, nil},
		{"a wildcard over the qualified name", method, "example.test/fixture/p.*.Get", true, nil},
		{"no match", function, "Open", false, nil},
		{"a malformed pattern", function, "[", false, path.ErrBadPattern},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := testCase.call.Matches(testCase.pattern)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Matches(%q) error = %v, want %v", testCase.pattern, err, testCase.wantErr)
			}

			if got != testCase.want {
				t.Fatalf("Matches(%q) = %t, want %t", testCase.pattern, got, testCase.want)
			}
		})
	}
}
