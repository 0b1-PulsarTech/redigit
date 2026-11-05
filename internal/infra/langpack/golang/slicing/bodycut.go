package slicing

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
	"github.com/wrapped-owls/redigit/internal/infra/sliceconf"
)

type bodyCut struct {
	fset     *token.FileSet
	fn       entities.Decl
	decl     *ast.FuncDecl
	stmts    []ast.Stmt
	elements []ast.Expr
	comments ast.CommentMap
	info     localTypes
	policy   sliceconf.Config
}

func (cut bodyCut) slice() indexing.Sliced {
	flow := readFlow(cut.stmts, cut.info)
	groups := newGroups(len(cut.stmts) + 1)
	head := len(cut.stmts)

	// A signature born alone would render as the whole function, since a render
	// that finds no statement of a function writes it whole. A function with
	// results is born with its final statement, one without with its first.
	anchor := 0
	if cut.decl.Type.Results != nil && len(cut.decl.Type.Results.List) > 0 {
		anchor = len(cut.stmts) - 1
	}

	groups.join(head, anchor)
	flow.joinDeclarations(groups)

	if cut.policy.Errcheck {
		cut.joinReassigns(flow, groups)
	}

	out := indexing.Sliced{Head: cut.head(), Members: cut.members()}
	out.Edges = cut.edges(flow)

	for index := range out.Members {
		out.Members[index].Atom = cut.atomOf(groups, index)
	}

	out.Head.Atom = cut.atomOf(groups, head)

	return out
}

func (cut bodyCut) atomOf(groups groups, index int) string {
	root := groups.find(index)
	if groups.size(root) < syntax.MinGroup {
		if index == len(cut.stmts) {
			return cut.fn.Atom
		}

		return ""
	}

	if headRoot := groups.find(len(cut.stmts)); root == headRoot && cut.fn.Atom != "" {
		return cut.fn.Atom
	}

	return cut.fn.Key.File + ":" + cut.fn.Key.Name + "#atom" + strconv.Itoa(root)
}

func (cut bodyCut) head() entities.Decl {
	head := cut.fn
	head.Span.End = cut.offset(cut.decl.Body.Lbrace) + 1

	return head
}

func (cut bodyCut) members() []entities.Decl {
	parts := make([]ast.Node, 0, max(len(cut.stmts), len(cut.elements)))

	if len(cut.elements) > 0 {
		for _, element := range cut.elements {
			parts = append(parts, element)
		}
	} else {
		for _, stmt := range cut.stmts {
			parts = append(parts, stmt)
		}
	}

	out := make([]entities.Decl, 0, len(parts))
	floor := cut.offset(cut.decl.Body.Lbrace) + 1

	for index, part := range parts {
		start, end := cut.offset(part.Pos()), cut.offset(part.End())

		for _, group := range cut.comments[part] {
			start = min(start, cut.offset(group.Pos()))
			end = max(end, cut.offset(group.End()))
		}

		start = max(start, floor)

		if index+1 < len(parts) {
			end = min(end, cut.offset(parts[index+1].Pos()))
		}

		floor = end

		out = append(out, entities.Decl{
			Key: entities.DeclKey{
				File: cut.fn.Key.File,
				Kind: entities.KindStmt,
				Name: syntax.StmtName(cut.fn.Key.Name, index),
			},
			Unit:   cut.fn.Unit,
			Span:   entities.Span{Start: start, End: end},
			IsTest: cut.fn.IsTest,
		})
	}

	return out
}

func (cut bodyCut) offset(pos token.Pos) int {
	return cut.fset.Position(pos).Offset
}

func (cut bodyCut) edges(flow bodyFlow) []entities.Edge {
	key := func(index int) entities.DeclKey {
		return entities.DeclKey{
			File: cut.fn.Key.File,
			Kind: entities.KindStmt,
			Name: syntax.StmtName(cut.fn.Key.Name, index),
		}
	}

	var out []entities.Edge

	add := func(from, to int) {
		out = append(out, entities.Edge{
			Dependent: key(from),
			Provider:  key(to),
			Reason:    entities.ReasonBody,
		})
	}

	for index := range cut.stmts {
		out = append(out, entities.Edge{
			Dependent: key(index),
			Provider:  cut.fn.Key,
			Reason:    entities.ReasonBody,
		})

		for _, to := range flow.needs(index) {
			add(index, to)
		}
	}

	return out
}
