package di

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

func typeID(ref facts.TypeRef) string {
	if ref.Name != "" {
		return ref.Name
	}

	if ref.Key.IsZero() {
		return ""
	}

	return ref.Key.String()
}

func matchesAny(call facts.Call, patterns []string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := call.Matches(pattern)
		if err != nil {
			return false, fmt.Errorf("match call: %w", err)
		}

		if matched {
			return true, nil
		}
	}

	return false, nil
}

func (cfg Config) injects(call facts.Call) bool {
	// Only a method-style API names its injector in the receiver; a plain function is never filtered.
	if call.Receiver.IsZero() {
		return true
	}

	return cfg.isInjector(call.Receiver.Name)
}

func (cfg Config) isInjector(name string) bool {
	name = strings.TrimPrefix(name, "*")
	short := name[strings.LastIndex(name, ".")+1:]

	return slices.ContainsFunc(cfg.InjectorTypes, func(pattern string) bool {
		full, _ := path.Match(pattern, name)
		bare, _ := path.Match(pattern, short)

		return full || bare
	})
}

func isRoot(key entities.DeclKey, roots []string) bool {
	if key.Kind != entities.KindFunc && key.Kind != entities.KindMethod {
		return false
	}

	names := []string{key.Name}
	if dir := path.Dir(key.File); dir != "." {
		names = append(names, dir+"."+key.Name)
	}

	return slices.ContainsFunc(roots, func(pattern string) bool {
		return slices.ContainsFunc(names, func(name string) bool {
			matched, _ := path.Match(pattern, name)

			return matched
		})
	})
}

func registers(cfg Config, call facts.Call) bool {
	found, err := matchesAny(call, cfg.RegisterFuncs)

	return err == nil && found && cfg.injects(call)
}
