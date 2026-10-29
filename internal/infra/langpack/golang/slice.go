package golang

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
)

const (
	// minStatements is the fewest statements worth cutting a body into.
	minStatements = 2
)

// stubImporter gives every import an empty package. The slicer needs only to
// tell one local from another, and a local is told apart whether or not the
// packages it talks to can be read.
type stubImporter struct{}

func (stubImporter) Import(path string) (*types.Package, error) {
	pkg := types.NewPackage(path, path[strings.LastIndex(path, "/")+1:])
	pkg.MarkComplete()

	return pkg, nil
}

// typeLocals resolves the identifiers of one file. Type errors are expected and
// ignored: every package outside the file is a stub, so most calls into one do
// not type, and the locals are still resolved.
func typeLocals(fset *token.FileSet, file *ast.File) *types.Info {
	info := &types.Info{
		Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{},
	}
	conf := types.Config{
		Importer:    stubImporter{},
		Error:       func(error) {},
		FakeImportC: true,
	}

	// The error is the point: the stubs make the check fail, and the objects are
	// recorded anyway.
	_, _ = conf.Check(file.Name.Name, fset, []*ast.File{file}, info)

	return info
}

// Slice cuts a function into its signature and its top-level statements.
//
// What keeps every intermediate state compilable is the dataflow of the body:
// a statement waits for the one that declares a local it reads and for the one
// that last wrote it, and a declaration is born with the first statement that
// reads what it declares, because Go refuses a local that is declared and not
// used. A function with results is born with its final statement, or it would
// stop compiling for want of a return.
func (Syntax) Slice(
	_ context.Context,
	fn entities.Decl,
	source []byte,
) (indexing.Sliced, bool, error) {
	if fn.Key.Kind != entities.KindFunc && fn.Key.Kind != entities.KindMethod {
		return indexing.Sliced{}, false, nil
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, fn.Key.File, source, parser.ParseComments)
	if err != nil {
		return indexing.Sliced{}, false, fmt.Errorf("parse %s to slice: %w", fn.Key.File, err)
	}

	decl := findFuncDecl(file, fn.Key)
	if decl == nil || decl.Body == nil {
		return indexing.Sliced{}, false, nil
	}

	cut := bodyCut{fset: fset, fn: fn, decl: decl, stmts: decl.Body.List}

	if elements, _ := syntax.ReturnedElements(decl.Body); len(elements) >= syntax.MinGroup {
		cut.elements = elements

		return cut.sliceElements(), true, nil
	}

	if !cuttable(decl.Body) {
		return indexing.Sliced{}, false, nil
	}

	cut.comments = ast.NewCommentMap(fset, file, file.Comments)
	cut.info = typeLocals(fset, file)

	return cut.slice(), true, nil
}

func findFuncDecl(file *ast.File, key entities.DeclKey) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc {
			continue
		}

		ident := syntax.FuncIdent(fn)
		if ident.Kind == key.Kind && ident.Name == key.Name {
			return fn
		}
	}

	return nil
}

// cuttable is whether the body can lose some of its statements and still
// compile. A label or a goto ties statements together by position, and a return
// before the last statement makes what follows unreachable only when all of it
// is present.
func cuttable(body *ast.BlockStmt) bool {
	if len(body.List) < minStatements {
		return false
	}

	for _, stmt := range body.List[:len(body.List)-1] {
		if _, isReturn := stmt.(*ast.ReturnStmt); isReturn {
			return false
		}
	}

	tied := false

	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.LabeledStmt:
			tied = true
		case *ast.BranchStmt:
			tied = tied || typed.Tok == token.GOTO
		}

		return !tied
	})

	return !tied
}

type bodyCut struct {
	fset     *token.FileSet
	fn       entities.Decl
	decl     *ast.FuncDecl
	stmts    []ast.Stmt
	elements []ast.Expr
	comments ast.CommentMap
	info     *types.Info
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

	out := indexing.Sliced{Head: cut.head(), Members: cut.members()}
	out.Edges = cut.edges(flow)

	for index := range out.Members {
		out.Members[index].Atom = cut.atomOf(groups, index)
	}

	out.Head.Atom = cut.atomOf(groups, head)

	return out
}

// atomOf names the group a piece is born with, empty for a piece that stands
// alone. The head keeps an atom the function already had.
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

// members are the statements in source order. A statement carries the comments
// attached to it, and its span never reaches into a neighbour's.
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

// edges are what a statement needs from the function it sits in: the
// signature, the statement that declares a local it touches, and the statement
// that last wrote a local it reads.
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

// sliceElements cuts a one-return body into its elements. The signature is
// born with the first, because a function with results needs its return.
func (cut bodyCut) sliceElements() indexing.Sliced {
	joined := newGroups(len(cut.elements) + 1)
	head := len(cut.elements)
	joined.join(head, 0)

	out := indexing.Sliced{Head: cut.head(), Members: cut.members()}

	for index := range out.Members {
		out.Edges = append(out.Edges, entities.Edge{
			Dependent: out.Members[index].Key,
			Provider:  cut.fn.Key,
			Reason:    entities.ReasonBody,
		})
		out.Members[index].Atom = cut.elementAtom(joined, index)
	}

	out.Head.Atom = cut.elementAtom(joined, head)

	return out
}

func (cut bodyCut) elementAtom(joined groups, index int) string {
	root := joined.find(index)
	if joined.size(root) < syntax.MinGroup {
		return ""
	}

	return cut.fn.Key.File + ":" + cut.fn.Key.Name + "#atom" + strconv.Itoa(root)
}
