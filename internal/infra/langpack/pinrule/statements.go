package pinrule

import (
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func withStatements(providers, decls []entities.Decl) []entities.Decl {
	statements := map[entities.DeclKey][]entities.Decl{}

	for _, decl := range decls {
		if head, isStmt := stmtHead(decl.Key); isStmt {
			statements[head] = append(statements[head], decl)
		}
	}

	expanded := make([]entities.Decl, 0, len(providers))
	seen := map[entities.DeclKey]bool{}
	add := func(decl entities.Decl) {
		if !seen[decl.Key] {
			seen[decl.Key] = true

			expanded = append(expanded, decl)
		}
	}

	for _, provider := range providers {
		add(provider)

		if provider.Key.Kind == entities.KindFunc || provider.Key.Kind == entities.KindMethod {
			for _, statement := range statements[functionID(provider.Key)] {
				add(statement)
			}
		}
	}

	return expanded
}

func functionID(key entities.DeclKey) entities.DeclKey {
	return entities.DeclKey{File: key.File, Kind: entities.KindFunc, Name: key.Name}
}

func stmtHead(key entities.DeclKey) (entities.DeclKey, bool) {
	cut := strings.LastIndex(key.Name, "#")
	if key.Kind != entities.KindStmt || cut < 0 {
		return entities.DeclKey{}, false
	}

	return entities.DeclKey{File: key.File, Kind: entities.KindFunc, Name: key.Name[:cut]}, true
}

func sameFunction(dependent, provider entities.DeclKey) bool {
	// a function does not wait for its own body, nor a statement for its sibling
	if provider.Kind != entities.KindStmt {
		return false
	}

	own := functionID(dependent)
	if head, isStmt := stmtHead(dependent); isStmt {
		own = head
	}

	head, _ := stmtHead(provider)

	return own == head
}
