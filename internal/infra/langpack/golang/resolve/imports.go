package resolve

import (
	"bytes"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func clausesOf(pkgs []*packages.Package, where *places) map[string][]site {
	// gopls reads a package clause as the name every import of the package spells, so a clause of
	// a file that declares nothing still has an answer. A file listed by a package and by its test
	// variant holds one clause.
	clauses, seen := map[string][]site{}, map[site]bool{}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			at, _, written := where.at(pkg.Fset, file.Name.Pos())
			if !written || seen[at] {
				continue
			}

			seen[at] = true

			clauses[pkg.PkgPath] = append(clauses[pkg.PkgPath], at)
		}
	}

	return clauses
}

func (found usage) addImports(pkgs []*packages.Package, where *places) {
	clauses := clausesOf(pkgs, where)
	known := map[site]map[entities.Location]bool{}
	roots := map[string]bool{}

	for _, pkg := range pkgs {
		roots[pkg.ID] = true
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, spec := range file.Imports {
				_, importAt, written := where.at(pkg.Fset, spec.Pos())
				imported, err := strconv.Unquote(spec.Path.Value)

				// An import of the test variant is the package's own external test, which gopls
				// leaves out of the clause's readers.
				dep := pkg.Imports[imported]

				if !written || err != nil || (dep != nil && dep.ID != dep.PkgPath) {
					continue
				}

				// This group met the package as export data only, so each of its files has
				// readers here that nobody sees, its package clauses among them.
				if dep != nil && !roots[dep.ID] {
					found.leaveUnread(dep, where)
				}

				for _, clause := range clauses[imported] {
					found.addReader(known, clause, importAt)
				}
			}
		}
	}
}

func (found usage) addReader(
	known map[site]map[entities.Location]bool,
	clause site,
	importAt entities.Location,
) {
	if known[clause] == nil {
		known[clause] = found.readersOf(clause)
	}

	if !known[clause][importAt] {
		known[clause][importAt] = true

		found.refs[clause] = append(found.refs[clause], importAt)
	}
}

func depsOf(pkgs []*packages.Package) map[string][]*packages.Package {
	roots, deps := map[string]bool{}, map[string][]*packages.Package{}

	for _, pkg := range pkgs {
		roots[pkg.ID] = true
	}

	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if !roots[pkg.ID] {
			deps[pkg.PkgPath] = append(deps[pkg.PkgPath], pkg)
		}
	})

	return deps
}

func (found usage) leaveWhole(dep *packages.Package, where *places) {
	for _, name := range dep.GoFiles {
		if rel := where.relative(name); rel != "" {
			found.unread[rel] = true
		}
	}
}

func (found usage) leaveDirected(dep *packages.Package, where *places) {
	for _, name := range dep.GoFiles {
		rel := where.relative(name)
		if rel == "" {
			continue
		}

		// A directive renames and shifts what export data records, so no line names its source.
		file, _ := where.text(rel)
		if bytes.Contains(file.text, []byte("//line ")) ||
			bytes.Contains(file.text, []byte("/*line ")) {
			found.unread[rel] = true
		}
	}
}

func (found usage) refuseTestClauses(pkgs []*packages.Package, where *places) {
	// A test file is no part of the package an importer sees, so its clause has readers no
	// other group's export data can place.
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			at, _, written := where.at(pkg.Fset, file.Name.Pos())
			if written && strings.HasSuffix(at.file, "_test.go") {
				line := pkg.Fset.PositionFor(file.Name.Pos(), false).Line
				found.lost[row{file: at.file, line: line}] = true
			}
		}
	}
}

func (found usage) leaveUnread(dep *packages.Package, where *places) {
	for _, name := range dep.GoFiles {
		rel := where.relative(name)
		if rel == "" {
			continue
		}

		// A declaration of the file is read here only through a use, which marks its own line;
		// the package clause is the one place every import reads without a use.
		file, _ := where.text(rel)
		fset := token.NewFileSet()

		head, err := parser.ParseFile(fset, rel, file.text, parser.PackageClauseOnly)
		if err != nil || head.Name == nil {
			found.unread[rel] = true

			continue
		}

		found.lost[row{file: rel, line: fset.Position(head.Name.Pos()).Line}] = true
	}
}

func (found usage) readersOf(clause site) map[entities.Location]bool {
	readers := make(map[entities.Location]bool, len(found.refs[clause]))

	for _, reader := range found.refs[clause] {
		readers[reader] = true
	}

	return readers
}
