package sql

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
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

func heavyHelpers(
	model sqlModel,
	helpers map[string]map[string]bool,
	tableTypes map[string]bool,
) map[string]bool {
	heavy := map[string]bool{}

	for changed := true; changed; {
		changed = false

		for file, idents := range helpers {
			if heavy[file] || !namesAny(idents, tableTypes, heavy, model.decls) {
				continue
			}

			heavy[file], changed = true, true
		}
	}

	return heavy
}

func namesAny(
	idents, tableTypes map[string]bool,
	heavy map[string]bool,
	byFile map[string][]entities.Decl,
) bool {
	for name := range tableTypes {
		if idents[name] {
			return true
		}
	}

	for file := range heavy {
		for _, decl := range byFile[file] {
			if decl.Key.Kind != entities.KindMethod && idents[decl.Key.Name] {
				return true
			}
		}
	}

	return false
}

func identsOf(snap entities.Snapshot, file string) map[string]bool {
	if !strings.HasSuffix(file, ".go") {
		return nil
	}

	source, err := declsource.ReadSource(snap, file)
	if err != nil {
		return nil
	}

	parsed, err := parser.ParseFile(token.NewFileSet(), file, source, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}

	idents := map[string]bool{}

	ast.Inspect(parsed, func(node ast.Node) bool {
		if ident, isIdent := node.(*ast.Ident); isIdent {
			idents[ident.Name] = true
		}

		return true
	})

	return idents
}

func configKeys(snap entities.Snapshot) []entities.DeclKey {
	var keys []entities.DeclKey

	for _, file := range snap.Files {
		switch path.Base(file) {
		case "sqlc.yaml", "sqlc.yml", "sqlc.json":
			if holdsAny(path.Dir(file), queryFiles(snap.Files)) {
				keys = append(keys, wholeFileKey(file))
			}
		}
	}

	return keys
}

func (model sqlModel) moduleKeys(snap entities.Snapshot) []entities.DeclKey {
	dirs := map[string]bool{model.ormDir: model.ormDir != ""}

	for _, file := range configKeys(snap) {
		dirs[path.Dir(file.File)] = true
	}

	var keys []entities.DeclKey

	for _, file := range slices.Sorted(maps.Keys(model.decls)) {
		if name := path.Base(file); (name == "go.mod" || name == "go.sum") && dirs[path.Dir(file)] {
			for _, decl := range model.decls[file] {
				keys = append(keys, decl.Key)
			}
		}
	}

	return keys
}

func checksumKeys(snap entities.Snapshot) []entities.DeclKey {
	var keys []entities.DeclKey

	for _, file := range snap.Files {
		if path.Base(file) == "atlas.sum" && holdsAny(path.Dir(file), migrationFiles(snap.Files)) {
			keys = append(keys, wholeFileKey(file))
		}
	}

	return keys
}

func holdsAny(dir string, files []string) bool {
	return slices.ContainsFunc(files, func(file string) bool {
		return dir == "." || strings.HasPrefix(file, dir+"/")
	})
}

func wholeFileKey(file string) entities.DeclKey {
	return entities.DeclKey{File: file, Kind: entities.KindOther, Name: syntax.FileDeclName}
}
