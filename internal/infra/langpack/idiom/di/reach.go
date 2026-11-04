package di

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

func reachable(graph entities.Graph, roots []string) map[entities.DeclKey]bool {
	// Nil when no root is configured, which means every site counts.
	if len(roots) == 0 {
		return nil
	}

	members := map[entities.DeclKey][]entities.DeclKey{}
	queue := []entities.DeclKey{}

	for _, decl := range graph.Decls() {
		key := decl.Key

		if key.Kind == entities.KindStmt {
			cut := lastHash(key.Name)
			for _, kind := range [...]entities.DeclKind{entities.KindFunc, entities.KindMethod} {
				head := entities.DeclKey{File: key.File, Kind: kind, Name: key.Name[:cut]}
				members[head] = append(members[head], key)
			}

			continue
		}

		if isRoot(key, roots) && !entities.IsTestSide(decl) {
			queue = append(queue, key)
		}
	}

	seen := map[entities.DeclKey]bool{}

	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]

		if seen[key] {
			continue
		}

		seen[key] = true

		queue = append(queue, members[key]...)

		for _, edge := range graph.Needs(key) {
			queue = append(queue, edge.Provider)
		}
	}

	return seen
}

func runByOthers(
	cfg Config,
	decls []entities.Decl,
	owners map[entities.DeclKey][]int,
	known idiom.Facts,
	seed func(entities.Decl) bool,
) map[entities.DeclKey]bool {
	ran := ranByCtors(owners, known)

	queue := bootSeeds(cfg, decls, owners, ran, seed)
	reached := map[entities.DeclKey]bool{}

	// A seed that a constructor also runs is shared the same as one it reaches.
	for _, key := range queue {
		if ran[key] {
			reached[key] = true
		}
	}

	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]

		for _, call := range known.Calls(key) {
			// A literal handed to a registration runs when the container builds it.
			if call.Within != nil && registers(cfg, *call.Within) {
				continue
			}

			isRegistration := registers(cfg, call)

			for pos, target := range append([]entities.DeclKey{call.CalleeKey}, call.FuncArgs...) {
				// A registered constructor passed as an argument is run by its registration;
				// one called directly runs outside the container, so its helpers count as shared.
				registered := pos > 0 && isRegistration && len(owners[target]) > 0
				if !ran[target] || registered || reached[target] {
					continue
				}

				reached[target] = true

				queue = append(queue, target)
			}
		}
	}

	return reached
}

func ranByCtors(owners map[entities.DeclKey][]int, known idiom.Facts) map[entities.DeclKey]bool {
	ran := map[entities.DeclKey]bool{}

	var queue []entities.DeclKey

	for ctor := range owners {
		if !ctor.IsZero() {
			ran[ctor] = true

			queue = append(queue, ctor)
		}
	}

	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]

		for _, call := range known.Calls(key) {
			for _, target := range targets(call) {
				if !ran[target] {
					ran[target] = true

					queue = append(queue, target)
				}
			}
		}
	}

	return ran
}

func bootSeeds(
	cfg Config, decls []entities.Decl, owners map[entities.DeclKey][]int,
	ran map[entities.DeclKey]bool, seed func(entities.Decl) bool,
) []entities.DeclKey {
	var out []entities.DeclKey

	for _, decl := range decls {
		// A configured root is boot code even when a constructor calls it, unless it is
		// a constructor itself, which a root glob such as New* would otherwise open.
		outside, boot := !ran[decl.Key], isBoot(cfg, owners, ran, decl.Key)

		if decl.Key.Kind == entities.KindStmt {
			// A statement stands for a func head and a method head; only the real one has run,
			// so the statement is outside only when none of them did.
			for _, head := range heads(decl.Key)[1:] {
				outside = outside && !ran[head]
			}
		}

		if (outside || boot) && seed(decl) && !entities.IsTestSide(decl) {
			out = append(out, decl.Key)
		}
	}

	return out
}

func isBoot(
	cfg Config, owners map[entities.DeclKey][]int, ran map[entities.DeclKey]bool,
	key entities.DeclKey,
) bool {
	return ran[key] && isRoot(key, cfg.Roots) && len(owners[key]) == 0
}
