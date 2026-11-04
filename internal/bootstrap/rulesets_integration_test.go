//go:build integration

package bootstrap

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

func orderWithGoDI(
	t *testing.T,
	files map[string]string,
	raws ...string,
) (entities.Graph, entities.Ordering) {
	t.Helper()

	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../infra/langpack/idiom/testdata/dimod")); err != nil {
		t.Fatalf("copy dimod: %v", err)
	}

	for name, content := range files {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}

		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	initFixtureRepo(t, dir)

	if len(raws) == 0 {
		raws = []string{`{"type":"go-di","registerFuncs":["Register*"],"resolveFuncs":["Get"]}`}
	}

	specs := make([]rules.Spec, 0, len(raws))
	for _, raw := range raws {
		specs = append(specs, rules.Spec{Type: "go-di", Raw: []byte(raw)})
	}

	built, err := compiledRegistry().Build(specs)
	if err != nil {
		t.Fatalf("Build() = %v", err)
	}

	pipeline, err := Start(t.Context(), Config{Repo: dir, Rev: "HEAD", Rules: built})
	if err != nil {
		t.Fatalf("Start() = %v", err)
	}

	defer func() { _ = pipeline.Close(context.WithoutCancel(t.Context())) }()

	result, err := pipeline.Graph(t.Context())
	if err != nil {
		t.Fatalf("Graph() = %v", err)
	}

	order, err := pipeline.Order(result.Graph)
	if err != nil {
		t.Fatalf("Order() = %v, want a plan with no cycle", err)
	}

	return result.Graph, order
}

type cycleCase struct {
	name  string
	files map[string]string
	raws  []string
	ctors []string
}

func requirePlacedBeforeSetup(t *testing.T, testCases []cycleCase) {
	t.Helper()

	appFile := "ctor/app/app.go"
	setup := entities.DeclKey{File: "ctor/boot/boot.go", Kind: entities.KindFunc, Name: "Setup"}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, order := orderWithGoDI(t, testCase.files, testCase.raws...)

			for _, name := range testCase.ctors {
				ctor := entities.DeclKey{File: appFile, Kind: entities.KindFunc, Name: name}
				if order.Of(ctor) > order.Of(setup) {
					t.Fatalf("%s at %d is placed after Setup at %d",
						name, order.Of(ctor), order.Of(setup))
				}
			}
		})
	}
}

func TestGoDIRuleNeverInventsACycleThroughAConstructor(t *testing.T) {
	t.Parallel()

	boot := `package boot

import (
	"example.com/dimod/ctor/app"
	"example.com/dimod/inj"
)

func Setup(in *inj.Injector) {
	inj.Register0(in, app.NewA)
	inj.Register0(in, app.NewB)
}
`

	requirePlacedBeforeSetup(t, []cycleCase{
		{
			name: "a resolve inside a registered constructor",
			files: map[string]string{
				"ctor/app/app.go": `package app

import "example.com/dimod/inj"

type B struct{}

func NewB() *B { return &B{} }

type A struct{ b *B }

func NewA(in *inj.Injector) *A { return &A{b: inj.Get[*B](in)} }
`,
				"ctor/boot/boot.go": strings.Replace(
					boot, "Register0(in, app.NewA)", "Register1(in, app.NewA)", 1,
				),
			},
			ctors: []string{"NewA", "NewB"},
		},
		{
			name: "a constructor reaching a resolve through an interface method",
			files: map[string]string{
				"ctor/app/app.go": `package app

import "example.com/dimod/inj"

var In *inj.Injector

type B struct{}

func NewB() *B { return &B{} }

type Svc interface{ Run() *B }

type A struct{}

func (a *A) Run() *B { return inj.Get[*B](In) }

func NewA() Svc { return &A{} }
`,
				"ctor/boot/boot.go": boot,
			},
			ctors: []string{"NewA", "NewB"},
		},
	})
}

func TestGoDIRuleNeverInventsACycleThroughAFunctionValue(t *testing.T) {
	t.Parallel()

	boot := `package boot

import (
	"example.com/dimod/ctor/app"
	"example.com/dimod/inj"
)

func Setup(in *inj.Injector) {
	_ = app.Routes
	inj.Register0(in, app.NewA)
	inj.Register0(in, app.NewB)
}
`

	requirePlacedBeforeSetup(t, []cycleCase{
		{
			name: "a function value list read by the setup and resolving in a handler",
			files: map[string]string{
				"ctor/app/app.go": `package app

import "example.com/dimod/inj"

var In *inj.Injector

type B struct{}

func NewB() *B { return &B{} }

type A struct{}

func NewA() *A { return &A{} }

func Handle() *B { return inj.Get[*B](In) }

var Routes = []func() *B{Handle}
`,
				"ctor/boot/boot.go": boot,
			},
			ctors: []string{"NewA", "NewB"},
		},
		{
			name: "a constructor storing a function value that resolves",
			files: map[string]string{
				"ctor/app/app.go": `package app

import "example.com/dimod/inj"

var In *inj.Injector

var Routes []int

type B struct{}

func NewB() *B { return &B{} }

type A struct{ handler func() *B }

func NewA() *A { return &A{handler: Handle} }

func Handle() *B { return inj.Get[*B](In) }
`,
				"ctor/boot/boot.go": boot,
			},
			ctors: []string{"NewA", "NewB"},
		},
	})
}

func TestGoDIRuleNeverInventsACycleBetweenRegistrations(t *testing.T) {
	t.Parallel()

	registerIn := func(function, call string) string {
		return `package boot

import (
	"example.com/dimod/ctor/app"
	"example.com/dimod/inj"
)

func ` + function + `(in *inj.Injector) {
` + call + `
}
`
	}

	requirePlacedBeforeSetup(t, []cycleCase{
		{
			name: "register helpers that need each other's types",
			files: map[string]string{
				"ctor/app/app.go": `package app

type X struct{ y *Y }

type Y struct{ x *X }

func NewX(y *Y) *X { return &X{y: y} }

func NewY(x *X) *Y { return &Y{x: x} }
`,
				"ctor/boot/x.go": registerIn("SetupX", "inj.Register1(in, app.NewX)"),
				"ctor/boot/y.go": registerIn("SetupY", "inj.Register1(in, app.NewY)"),
			},
		},
		{
			name: "two go-di rules whose edges only close a cycle together",
			files: map[string]string{
				"ctor/app/app.go": `package app

type B struct{}

func NewB() *B { return &B{} }

type D struct{}

func NewD(count int) *D { return &D{} }
`,
				"ctor/boot/a.go": registerIn("SetupA",
					"inj.Register0(in, app.NewB)\n_ = inj.Get[*app.D](in)"),
				"ctor/boot/boot.go": registerIn("Setup",
					"inj.Register1(in, app.NewD)\n_ = inj.Get[*app.B](in)"),
			},
			raws: []string{
				`{"type":"go-di","registerFuncs":["Register0"],"resolveFuncs":["Get"]}`,
				`{"type":"go-di","registerFuncs":["Register1"],"resolveFuncs":["Get"]}`,
			},
			ctors: []string{"NewD"},
		},
	})
}

type helperCase struct {
	name   string
	files  map[string]string
	helper entities.DeclKey
}

func requireRegistrationKept(t *testing.T, testCases []helperCase) {
	t.Helper()

	const runFile = `package app

import "example.com/dimod/inj"

type Z struct{}

func NewZ() *Z { return &Z{} }

func Run(in *inj.Injector) *Z { return inj.Get[*Z](in) }
`

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			files := maps.Clone(testCase.files)
			files["app/run.go"] = runFile

			graph, _ := orderWithGoDI(t, files)
			run := entities.DeclKey{File: "app/run.go", Kind: entities.KindFunc, Name: "Run"}
			want := entities.Edge{
				Dependent: run, Provider: testCase.helper, Reason: entities.ReasonRegistered,
			}

			if !slices.Contains(graph.Edges(), want) {
				t.Fatalf("graph holds no %v; the resolve site lost its registration", want)
			}
		})
	}
}

func TestGoDIRuleKeepsARegistrationInAPackageMainHelper(t *testing.T) {
	t.Parallel()

	testCases := []helperCase{
		{
			name: "a setup helper in its own file of package main",
			files: map[string]string{
				"cmd/app/main.go": `package main

import (
	"example.com/dimod/app"
	"example.com/dimod/inj"
)

func main() {
	in := setup()
	app.Run(in)
}

var _ = inj.New
`,
				"cmd/app/setup.go": `package main

import (
	"example.com/dimod/app"
	"example.com/dimod/inj"
)

func setup() *inj.Injector {
	in := inj.New()
	inj.Register0(in, app.NewZ)

	return in
}
`,
			},
			helper: entities.DeclKey{
				File: "cmd/app/setup.go", Kind: entities.KindFunc, Name: "setup",
			},
		},
		{
			name: "a setup helper beside func main",
			files: map[string]string{
				"cmd/app/main.go": `package main

import (
	"example.com/dimod/app"
	"example.com/dimod/inj"
)

func main() {
	in := setup()
	app.Run(in)
}

func setup() *inj.Injector {
	in := inj.New()
	inj.Register0(in, app.NewZ)

	return in
}
`,
			},
			helper: entities.DeclKey{
				File: "cmd/app/main.go", Kind: entities.KindFunc, Name: "setup",
			},
		},
	}

	requireRegistrationKept(t, testCases)
}

func TestStartChecksTheGoDIPaths(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		paths   string
		wantErr error
	}{
		{name: "a path matching nothing", paths: "nowhere/", wantErr: rules.ErrUnmatchedGlob},
		{name: "a path matching a directory", paths: "boot/"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			raw := `{"type":"go-di","registerFuncs":["R*"],"paths":["` + testCase.paths + `"]}`
			dir, pipeline, err := startDimod(t, []rules.Spec{{Type: "go-di", Raw: []byte(raw)}})

			if pipeline != nil {
				_ = pipeline.Close(context.WithoutCancel(t.Context()))
			}

			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Start() err = %v, want %v", err, testCase.wantErr)
			}

			if got := fixtureWorktreeCount(t, dir); err != nil && got != 1 {
				t.Fatalf("worktree count = %d, want 1: a checkout was made", got)
			}
		})
	}
}
