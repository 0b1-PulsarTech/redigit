package slicing

import (
	"context"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
	"github.com/wrapped-owls/redigit/internal/infra/sliceconf"
)

func TestSliceFindsALocalThatTheTypeCheckCannotResolve(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		body string
		want []int
	}{
		{
			"a local read only as the key of an imported map literal",
			"key := f()\n\thead := http.Header{key: {g()}}\n\tsink(head)",
			[]int{0, 0, 0, 0},
		},
		{
			"a local read only as the size of make of an imported type",
			"size := f()\n\tbuf := make(http.Header, size)\n\tsink(buf)",
			[]int{0, 0, 0, 0},
		},
		{
			"a shadowing name in a closure is not a read of the outer local",
			"key := f()\n\tfunc() {\n\t\tkey := 1\n\t\t_ = http.Header{key: nil}\n\t}()\n\tsink(key)",
			[]int{0, -1, 0, 0},
		},
		{
			"a selector of the same name is not a read of the local",
			"Header := f()\n\tg(http.Header)\n\tsink(Header)",
			[]int{0, -1, 0, 0},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := []byte(
				"package app\n\nimport \"net/http\"\n\nfunc Index() {\n\t" + testCase.body + "\n}\n",
			)

			decls, err := syntax.Declarations(context.Background(), "app/app.go", source)
			if err != nil {
				t.Fatalf("Declarations() error = %v", err)
			}

			for _, decl := range decls {
				if decl.Key.Name != "Index" {
					continue
				}

				sliced, ok, err := Slice(context.Background(), decl, source, sliceconf.Config{})
				if err != nil || !ok {
					t.Fatalf("Slice() ok = %v, error = %v", ok, err)
				}

				if got := atomShape(sliced); !slices.Equal(got, testCase.want) {
					t.Fatalf("atoms = %v, want %v", got, testCase.want)
				}
			}
		})
	}
}
