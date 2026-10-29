package slicing

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
)

type bodyFlow struct {
	declares []map[types.Object]bool
	writes   []map[types.Object]bool
	reads    []map[types.Object]bool
	declared map[types.Object]int
	info     *types.Info
}

func readFlow(stmts []ast.Stmt, info *types.Info) bodyFlow {
	flow := bodyFlow{
		declares: make([]map[types.Object]bool, len(stmts)),
		writes:   make([]map[types.Object]bool, len(stmts)),
		reads:    make([]map[types.Object]bool, len(stmts)),
		declared: map[types.Object]int{},
		info:     info,
	}

	for index, stmt := range stmts {
		flow.declares[index] = declaredBy(stmt, info)
		flow.writes[index] = map[types.Object]bool{}
		flow.reads[index] = map[types.Object]bool{}

		for obj := range flow.declares[index] {
			flow.declared[obj] = index
			flow.writes[index][obj] = true
		}
	}

	for index, stmt := range stmts {
		flow.visit(index, stmt)
	}

	return flow
}

func declaredBy(stmt ast.Stmt, info *types.Info) map[types.Object]bool {
	out := map[types.Object]bool{}

	add := func(ident *ast.Ident) {
		if obj := info.Defs[ident]; obj != nil {
			out[obj] = true
		}
	}

	switch typed := stmt.(type) {
	case *ast.AssignStmt:
		if typed.Tok != token.DEFINE {
			return out
		}

		for _, lhs := range typed.Lhs {
			if ident, isIdent := lhs.(*ast.Ident); isIdent {
				add(ident)
			}
		}
	case *ast.DeclStmt:
		gen, isGen := typed.Decl.(*ast.GenDecl)
		if !isGen {
			return out
		}

		for _, spec := range gen.Specs {
			switch named := spec.(type) {
			case *ast.ValueSpec:
				for _, ident := range named.Names {
					add(ident)
				}
			case *ast.TypeSpec:
				add(named.Name)
			}
		}
	}

	return out
}

func (flow bodyFlow) visit(index int, stmt ast.Stmt) {
	targets := map[*ast.Ident]bool{}

	ast.Inspect(stmt, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign || (assign.Tok != token.ASSIGN && assign.Tok != token.DEFINE) {
			return true
		}

		for _, lhs := range assign.Lhs {
			if ident, isIdent := lhs.(*ast.Ident); isIdent {
				targets[ident] = true
			}
		}

		return true
	})

	ast.Inspect(stmt, func(node ast.Node) bool {
		ident, isIdent := node.(*ast.Ident)
		if !isIdent {
			return true
		}

		obj := flow.objectOf(ident)
		if _, known := flow.declared[obj]; obj == nil || !known {
			return true
		}

		if targets[ident] {
			flow.writes[index][obj] = true
		} else {
			flow.reads[index][obj] = true
		}

		return true
	})
}

func (flow bodyFlow) objectOf(ident *ast.Ident) types.Object {
	if obj := flow.info.Defs[ident]; obj != nil {
		return obj
	}

	return flow.info.Uses[ident]
}

func (flow bodyFlow) needs(index int) []int {
	var out []int

	touched := map[types.Object]bool{}
	for obj := range flow.writes[index] {
		touched[obj] = true
	}

	for obj := range flow.reads[index] {
		touched[obj] = true
	}

	for obj := range touched {
		if declared := flow.declared[obj]; declared < index {
			out = append(out, declared)
		}
	}

	for obj := range flow.reads[index] {
		for before := index - 1; before >= 0; before-- {
			if flow.writes[before][obj] {
				out = append(out, before)

				break
			}
		}
	}

	slices.Sort(out)

	return slices.Compact(out)
}

func (flow bodyFlow) joinDeclarations(groups groups) {
	for index := range flow.declares {
		for obj := range flow.declares[index] {
			flow.joinReader(groups, index, obj)
		}
	}
}

func (flow bodyFlow) joinReader(groups groups, declaration int, obj types.Object) {
	first := -1

	for index := declaration + 1; index < len(flow.reads); index++ {
		if !flow.reads[index][obj] {
			continue
		}

		if groups.find(index) == groups.find(declaration) {
			return
		}

		if first < 0 {
			first = index
		}
	}

	if first >= 0 {
		groups.join(declaration, first)
	}
}

type groups struct{ parent []int }

func newGroups(size int) groups {
	parent := make([]int, size)
	for index := range parent {
		parent[index] = index
	}

	return groups{parent: parent}
}

func (set groups) find(index int) int {
	for set.parent[index] != index {
		set.parent[index] = set.parent[set.parent[index]]
		index = set.parent[index]
	}

	return index
}

func (set groups) join(left, right int) {
	rootA, rootB := set.find(left), set.find(right)
	if rootA != rootB {
		set.parent[max(rootA, rootB)] = min(rootA, rootB)
	}
}

func (set groups) size(root int) int {
	count := 0

	for index := range set.parent {
		if set.find(index) == root {
			count++
		}
	}

	return count
}
