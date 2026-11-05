package sql

import (
	"maps"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type sqlGenesis struct {
	table   *sqlTable
	query   *sqlQuery
	members []entities.DeclKey
}

func genesisOf(snap entities.Snapshot, model sqlModel) (sqlGenesis, bool) {
	infra, heavy := model.ormInfra(snap)

	for _, name := range model.order {
		table := model.tables[name]

		query, found := model.queryReading(name)
		if !found || len(table.refs) > 0 || !model.standsAlone(snap, table, heavy) {
			continue
		}

		genesis := sqlGenesis{table: table, query: query}
		// The first table needs the whole schema tooling set in one commit, or the tools reject the tree.
		genesis.members = slices.Concat(
			model.dbInit, infra, query.generated, []entities.DeclKey{query.block},
			configKeys(snap), checksumKeys(snap), model.moduleKeys(snap),
		)

		return genesis, true
	}

	return sqlGenesis{}, false
}

func (model sqlModel) queryReading(name string) (*sqlQuery, bool) {
	for _, query := range model.queries {
		if len(query.tables) == 1 && query.tables[0] == name {
			return query, true
		}
	}

	return nil, false
}

func (model sqlModel) standsAlone(
	snap entities.Snapshot,
	table *sqlTable,
	heavy map[string]bool,
) bool {
	if table.entFile == "" {
		return false
	}

	idents := identsOf(snap, table.entFile)

	for _, other := range model.tables {
		if other != table && other.entType.Name != "" && idents[other.entType.Name] {
			return false
		}
	}

	for name := range heavy {
		if idents[name] {
			return false
		}
	}

	return true
}

func (model sqlModel) ormInfra(snap entities.Snapshot) ([]entities.DeclKey, map[string]bool) {
	if model.ormDir == "" {
		return nil, nil
	}

	tableFiles := map[string]bool{}
	tableTypes := map[string]bool{}

	for _, table := range model.tables {
		tableFiles[table.entFile] = true
		tableTypes[table.entType.Name] = true
	}

	helpers := map[string]map[string]bool{}

	for file := range model.decls {
		if strings.HasPrefix(file, model.ormDir+"/") && !tableFiles[file] {
			helpers[file] = identsOf(snap, file)
		}
	}

	heavy := heavyHelpers(model, helpers, tableTypes)

	var infra []entities.DeclKey

	for _, file := range slices.Sorted(maps.Keys(helpers)) {
		if heavy[file] {
			continue
		}

		for _, decl := range model.decls[file] {
			infra = append(infra, decl.Key)
		}
	}

	names := map[string]bool{}

	for file := range heavy {
		for _, decl := range model.decls[file] {
			if decl.Key.Kind != entities.KindMethod {
				names[decl.Key.Name] = true
			}
		}
	}

	return infra, names
}
