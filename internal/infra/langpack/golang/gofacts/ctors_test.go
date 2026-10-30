package gofacts

import (
	"errors"
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func ctorFiles() map[string]string {
	return map[string]string{
		"a.go": `package fixture

import "net/http"

type Store interface{ Load() }

type Config struct{}

type Option func(*Config)

type Box[T any] struct{ item T }

var Answer = 42

func NewNone() *Config { return nil }

func NewPointer(cfg *Config) {}

func NewInterface(store Store) {}

func NewVariadic(opts ...Option) {}

func NewGeneric[T any](item T, box Box[T]) {}

func NewParam[T any](item T) {}

func NewNames(names ...string) {}

func NewMap(index map[string]Config) {}

func NewFunc(build func(Config) Store) {}

func NewExternal(client *http.Client, name string) {}

type Holder struct{}

func (Holder) Use(cfg Config, store Store) {}
`,
	}
}

func TestIndexCtorArgTypes(t *testing.T) {
	t.Parallel()

	fix := indexOf(t, ctorFiles())

	config := typeKey("Config")
	store := typeKey("Store")

	testCases := []struct {
		name string
		fn   entities.DeclKey
		want []facts.TypeRef
	}{
		{"no parameters", funcKey("a.go", "NewNone"), nil},
		{
			"a pointer",
			funcKey("a.go", "NewPointer"),
			[]facts.TypeRef{{Key: config, Name: "*" + module + ".Config"}},
		},
		{
			"an interface",
			funcKey("a.go", "NewInterface"),
			[]facts.TypeRef{{Key: store, Name: module + ".Store"}},
		},
		{
			"a variadic names the element's declaration",
			funcKey("a.go", "NewVariadic"),
			[]facts.TypeRef{{Key: typeKey("Option"), Name: "[]" + module + ".Option"}},
		},
		{
			"a type parameter and an instantiation of a generic type",
			funcKey("a.go", "NewGeneric"),
			[]facts.TypeRef{
				{Name: "T"},
				{Key: typeKey("Box"), Name: module + ".Box[T]"},
			},
		},
		{
			"external types have no key",
			funcKey("a.go", "NewExternal"),
			[]facts.TypeRef{{Name: "*net/http.Client"}, {Name: "string"}},
		},
		{
			"a method takes what follows its receiver",
			methodKey("Holder.Use"),
			[]facts.TypeRef{
				{Key: config, Name: module + ".Config"},
				{Key: store, Name: module + ".Store"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := fix.index.CtorArgTypes(testCase.fn)
			if err != nil {
				t.Fatalf("CtorArgTypes(%s) error = %v", testCase.fn, err)
			}

			if len(got) == 0 && len(testCase.want) == 0 {
				return
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("CtorArgTypes(%s) = %+v, want %+v", testCase.fn, got, testCase.want)
			}
		})
	}
}

func TestIndexCtorArgTypesKeepsOnlyNamedDeclarations(t *testing.T) {
	t.Parallel()

	fix := indexOf(t, ctorFiles())

	testCases := []struct {
		name string
		fn   string
		want []facts.TypeRef
	}{
		{"a type parameter alone has a name and no key", "NewParam", []facts.TypeRef{{Name: "T"}}},
		{"a variadic of a builtin is a slice", "NewNames", []facts.TypeRef{{Name: "[]string"}}},
		{
			"a map is not unwrapped to its element",
			"NewMap",
			[]facts.TypeRef{{Name: "map[string]" + module + ".Config"}},
		},
		{
			"a func type is not unwrapped to its parameters",
			"NewFunc",
			[]facts.TypeRef{{Name: "func(" + module + ".Config) " + module + ".Store"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := fix.index.CtorArgTypes(funcKey("a.go", testCase.fn))
			if err != nil {
				t.Fatalf("CtorArgTypes(%s) error = %v", testCase.fn, err)
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("CtorArgTypes(%s) = %+v, want %+v", testCase.fn, got, testCase.want)
			}
		})
	}
}

func TestIndexCtorArgTypesRefusesWhatIsNotAFunc(t *testing.T) {
	t.Parallel()

	fix := indexOf(t, ctorFiles())

	testCases := []struct {
		name string
		fn   entities.DeclKey
	}{
		{"a value", testkit.DeclKey("a.go", entities.KindValue, "Answer")},
		{"a type", typeKey("Config")},
		{"an unknown declaration", funcKey("a.go", "Missing")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := fix.index.CtorArgTypes(testCase.fn)
			if !errors.Is(err, ErrNotFunc) || got != nil {
				t.Fatalf("CtorArgTypes(%s) = %v, %v, want ErrNotFunc", testCase.fn, got, err)
			}
		})
	}
}
