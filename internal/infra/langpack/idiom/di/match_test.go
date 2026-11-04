package di

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

func TestConfigInjects(t *testing.T) {
	t.Parallel()

	plain := register(fnKey("Boot"), "Register", "p.A")
	onInjector := methodOn(plain, "x/di.Injector")
	onOther := methodOn(plain, "x/db.Pool")

	testCases := []struct {
		name string
		cfg  Config
		call facts.Call
		want bool
	}{
		{name: "a plain function is never filtered", cfg: Config{}, call: plain, want: true},
		{
			name: "a plain function stays with injector types set",
			cfg:  Config{InjectorTypes: []string{"Injector"}}, call: plain, want: true,
		},
		{
			name: "a receiver call is not a DI call without injector types",
			cfg:  Config{}, call: onOther, want: false,
		},
		{
			name: "a call on a listed injector is a DI call",
			cfg:  Config{InjectorTypes: []string{"Injector"}}, call: onInjector, want: true,
		},
		{
			name: "a call on another receiver is not a DI call",
			cfg:  Config{InjectorTypes: []string{"Injector"}}, call: onOther, want: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.cfg.injects(testCase.call); got != testCase.want {
				t.Fatalf("injects() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestIsRoot(t *testing.T) {
	t.Parallel()

	runAt := func(file string) entities.DeclKey {
		return entities.DeclKey{File: file, Kind: entities.KindFunc, Name: "run"}
	}
	app, tool := runAt("cmd/app/main.go"), runAt("tools/gen/main.go")
	typed := entities.DeclKey{File: "a/a.go", Kind: entities.KindType, Name: "run"}

	testCases := []struct {
		name  string
		key   entities.DeclKey
		roots []string
		want  bool
	}{
		{
			name: "a bare name matches in any directory",
			key:  app, roots: []string{"run"}, want: true,
		},
		{
			name: "a qualified root matches its directory",
			key:  app, roots: []string{"cmd/app.run"}, want: true,
		},
		{
			name: "a qualified root stays out of another directory",
			key:  tool, roots: []string{"cmd/app.run"},
		},
		{
			name: "a pattern may span the qualified name",
			key:  app, roots: []string{"cmd/*.r*"}, want: true,
		},
		{
			name: "a file at the module root keeps its bare name",
			key:  runAt("main.go"), roots: []string{"run"}, want: true,
		},
		{name: "a type is never a root", key: typed, roots: []string{"run"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := isRoot(testCase.key, testCase.roots); got != testCase.want {
				t.Fatalf("isRoot(%v, %v) = %v, want %v",
					testCase.key, testCase.roots, got, testCase.want)
			}
		})
	}
}

func TestRegisters(t *testing.T) {
	t.Parallel()

	site := fnKey("Boot")
	cfg := Config{RegisterFuncs: []string{"*Register"}, InjectorTypes: []string{"Injector"}}
	plain := register(site, "Register", "p.A")
	onInjector, onOther := methodOn(plain, "x/di.Injector"), methodOn(plain, "x/p.Cache")
	testCases := []struct {
		name string
		cfg  Config
		call facts.Call
		want bool
	}{
		{name: "a register function", cfg: cfg, call: plain, want: true},
		{name: "a method on an injector", cfg: cfg, call: onInjector, want: true},
		{name: "a method on another type", cfg: cfg, call: onOther},
		{name: "a function that is no register", cfg: cfg, call: get(site, "Get", "p.A")},
		{
			name: "a pattern that cannot match",
			cfg:  Config{RegisterFuncs: []string{"Reg[ister"}},
			call: register(site, "Register", "p.A"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := registers(testCase.cfg, testCase.call); got != testCase.want {
				t.Fatalf("registers(%+v) = %v, want %v", testCase.call, got, testCase.want)
			}
		})
	}
}
