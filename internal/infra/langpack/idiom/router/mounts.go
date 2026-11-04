package router

import (
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

type Mount struct {
	Fn     entities.DeclKey
	Caller entities.DeclKey
}

func findMounts(
	cfg Config,
	calls []facts.Call,
	bodies idiom.Facts,
	routers map[string]bool,
) []Mount {
	var (
		out    []Mount
		seen   = map[entities.DeclKey]bool{}
		memory = map[entities.DeclKey]bool{}
	)

	for _, call := range calls {
		// A caller that is a whole func is born once, so a mount cannot meet it.
		if call.Site.Kind != entities.KindStmt {
			continue
		}

		for _, target := range mountTargets(call, routers) {
			if seen[target] || isSelf(call.Site, target) ||
				strings.HasSuffix(target.File, "_test.go") {
				continue
			}

			registers, known := memory[target]
			if !known {
				registers = registersRoutes(cfg, bodies.Calls(target))
				memory[target] = registers
			}

			if !registers {
				continue
			}

			seen[target] = true

			out = append(out, Mount{Fn: target, Caller: call.Site})
		}
	}

	return out
}

func mountTargets(call facts.Call, routers map[string]bool) []entities.DeclKey {
	targets := call.FuncArgs

	// A method of the router type is the router's own API, not a mount made of its routes.
	own := call.Receiver.Name != "" && routers[call.Receiver.Name]
	if !call.CalleeKey.IsZero() && !own {
		targets = append([]entities.DeclKey{call.CalleeKey}, targets...)
	}

	return targets
}

func isSelf(site, target entities.DeclKey) bool {
	return slices.Contains(heads(site), target)
}

func registersRoutes(cfg Config, calls []facts.Call) bool {
	for _, call := range calls {
		if found, err := matchesAny(call, cfg.Handle); err == nil && found {
			return true
		}

		if found, err := matchesAny(call, cfg.Mount); err == nil && found {
			return true
		}
	}

	return false
}

func unmetMounts(
	cfg Config,
	calls []facts.Call,
	bodies idiom.Facts,
	routers map[string]bool,
	met []Mount,
) int {
	known := map[entities.DeclKey]bool{}
	for _, mount := range met {
		known[mount.Fn] = true
	}

	unmet := 0

	for _, call := range calls {
		if call.Site.Kind == entities.KindStmt {
			continue
		}

		for _, target := range mountTargets(call, routers) {
			if known[target] || isSelf(call.Site, target) ||
				strings.HasSuffix(target.File, "_test.go") {
				continue
			}

			known[target] = true

			if registersRoutes(cfg, bodies.Calls(target)) {
				unmet++
			}
		}
	}

	return unmet
}
