package typed

import (
	"context"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
)

func TestPositionsDeclAtConcurrent(t *testing.T) {
	root := t.TempDir()

	testCases := []struct {
		name    string
		files   []string
		workers int
	}{
		{name: "one file repeated", files: []string{"a.go"}, workers: 8},
		{name: "distinct files", files: []string{"a.go", "b.go", "c.go", "d.go"}, workers: 8},
		{name: "outside the root", files: []string{"../out.go", "a.go"}, workers: 8},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fset := token.NewFileSet()
			bases := make([]token.Pos, 0, len(testCase.files))

			for _, name := range testCase.files {
				file := fset.AddFile(filepath.Join(root, name), -1, 10)
				bases = append(bases, token.Pos(file.Base()))
			}

			shared := NewPositions(root, NewDeclLocator([]entities.Decl{}))
			copied := shared

			var group sync.WaitGroup

			for worker := range testCase.workers {
				where := shared
				if worker%2 == 1 {
					where = copied
				}

				group.Go(func() {
					for _, base := range bases {
						where.DeclAt(fset, base)
					}
				})
			}

			group.Wait()
		})
	}
}

func objectFixture(t *testing.T) (Positions, []entities.Decl, *packages.Package) {
	t.Helper()

	snap, decls := testkit.Fixture(t, map[string]string{
		"a/a.go": "package a\n\ntype T struct{}\n\nfunc (*T) M() {}\n\nfunc (T) V() {}\n\n" +
			"type G[X any] struct{}\n\nfunc (G[X]) Get() {}\n\ntype I interface{ Do() }\n\n" +
			"type Alias = T\n\nvar Var = 1\n\nconst Const = 2\n\nfunc F() {}\n\n" +
			"type TA = T\n\nfunc (TA) AM() {}\n\ntype PA = *T\n\nfunc (PA) PM() {}\n\n" +
			"func (*G[X]) Ptr() {}\n",
		"b/b.go": "package b\n\nimport \"example.test/fixture/a\"\n\nvar _ = a.F\n\nfunc Own() {}\n\n" +
			"type OwnType struct{}\n\ntype S struct{ F interface{ Fld() } }\n",
	})

	pkgs, err := typeCheck(
		context.Background(),
		snap.Root,
		loadPass{patterns: []string{"example.test/fixture/b"}},
	)
	if err != nil || len(pkgs) != 1 {
		t.Fatalf("typeCheck() = %d packages, error %v", len(pkgs), err)
	}

	where := NewPositions(snap.Root, NewDeclLocator(decls)).Parsing(pkgs)

	return where, decls, pkgs[0]
}

func memberOf(t *testing.T, pkg *types.Package, owner, member string) types.Object {
	t.Helper()

	if owner == "" {
		return pkg.Scope().Lookup(member)
	}

	found, _, _ := types.LookupFieldOrMethod(
		pkg.Scope().Lookup(owner).Type(), true, pkg, member,
	)

	return found
}

func anonymousMethod(t *testing.T, pkg *types.Package, owner, field string) types.Object {
	t.Helper()

	holder, isStruct := pkg.Scope().Lookup(owner).Type().Underlying().(*types.Struct)
	if !isStruct {
		t.Fatalf("%s is not a struct", owner)
	}

	for member := range holder.Fields() {
		if member.Name() == field {
			return member.Type().Underlying().(*types.Interface).ExplicitMethod(0)
		}
	}

	t.Fatalf("%s has no field %s", owner, field)

	return nil
}

func TestPositionsObjectAtKeysAnExportDataObjectByItsName(t *testing.T) {
	t.Parallel()

	where, _, root := objectFixture(t)
	imported := root.Types.Imports()[0]

	testCases := []struct {
		name   string
		member string
		want   entities.DeclKey
	}{
		{
			name:   "a func",
			member: "F",
			want:   testkit.DeclKey("a/a.go", entities.KindFunc, "F"),
		},
		{
			name:   "a type",
			member: "T",
			want:   testkit.DeclKey("a/a.go", entities.KindType, "T"),
		},
		{
			name:   "an alias",
			member: "Alias",
			want:   testkit.DeclKey("a/a.go", entities.KindType, "Alias"),
		},
		{
			name:   "a var",
			member: "Var",
			want:   testkit.DeclKey("a/a.go", entities.KindValue, "Var"),
		},
		{
			name:   "a const",
			member: "Const",
			want:   testkit.DeclKey("a/a.go", entities.KindValue, "Const"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, found := where.ObjectAt(root.Fset, imported.Scope().Lookup(testCase.member))
			if !found || got != testCase.want {
				t.Fatalf("ObjectAt() = %v, %v, want %v", got, found, testCase.want)
			}
		})
	}
}

func TestPositionsObjectAtKeysAnExportDataMethodByItsOwner(t *testing.T) {
	t.Parallel()

	where, _, root := objectFixture(t)
	imported := root.Types.Imports()[0]

	testCases := []struct {
		name   string
		owner  string
		member string
		want   entities.DeclKey
	}{
		{
			name:   "a pointer method",
			owner:  "T",
			member: "M",
			want:   testkit.DeclKey("a/a.go", entities.KindMethod, "T.M"),
		},
		{
			name:   "a value method",
			owner:  "T",
			member: "V",
			want:   testkit.DeclKey("a/a.go", entities.KindMethod, "T.V"),
		},
		{
			name:   "a generic method",
			owner:  "G",
			member: "Get",
			want:   testkit.DeclKey("a/a.go", entities.KindMethod, "G.Get"),
		},
		{
			name:   "a generic pointer method",
			owner:  "G",
			member: "Ptr",
			want:   testkit.DeclKey("a/a.go", entities.KindMethod, "G.Ptr"),
		},
		{
			name:   "an alias receiver",
			owner:  "T",
			member: "AM",
			want:   testkit.DeclKey("a/a.go", entities.KindMethod, "TA.AM"),
		},
		{
			name:   "a pointer alias receiver",
			owner:  "T",
			member: "PM",
			want:   testkit.DeclKey("a/a.go", entities.KindMethod, "PA.PM"),
		},
		{
			name:   "an interface method",
			owner:  "I",
			member: "Do",
			want:   testkit.DeclKey("a/a.go", entities.KindType, "I"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			obj := memberOf(t, imported, testCase.owner, testCase.member)
			if fn, isFunc := obj.(*types.Func); isFunc {
				obj = fn.Origin()
			}

			got, found := where.ObjectAt(root.Fset, obj)
			if !found || got != testCase.want {
				t.Fatalf("ObjectAt() = %v, %v, want %v", got, found, testCase.want)
			}
		})
	}
}

func TestPositionsObjectAtRefusesWhatItCannotKeyExactly(t *testing.T) {
	t.Parallel()

	where, decls, root := objectFixture(t)
	imported := root.Types.Imports()[0]
	elsewhere := NewPositions(t.TempDir(), NewDeclLocator(decls)).Parsing(nil)
	without := NewPositions(where.root, NewDeclLocator(slices.DeleteFunc(
		slices.Clone(decls),
		func(decl entities.Decl) bool { return decl.Key.Name == "F" },
	))).Parsing(nil)
	twice := NewPositions(where.root, NewDeclLocator(append(slices.Clone(decls), decls...))).
		Parsing(nil)

	testCases := []struct {
		name  string
		where Positions
		pkg   *types.Package
		obj   string
	}{
		{name: "a name the tree lacks", where: without, pkg: imported, obj: "F"},
		{name: "a file outside the tree", where: elsewhere, pkg: imported, obj: "F"},
		{name: "a key the tree holds twice", where: twice, pkg: imported, obj: "F"},
		{name: "an object no file declares", where: where, pkg: types.Unsafe, obj: "Pointer"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			obj := testCase.pkg.Scope().Lookup(testCase.obj)

			got, found := testCase.where.ObjectAt(root.Fset, obj)
			if found || got != (entities.DeclKey{}) {
				t.Fatalf("ObjectAt() = %v, %v, want the zero key and false", got, found)
			}
		})
	}
}

func TestPositionsObjectAtOfAParsedFileIsDeclAt(t *testing.T) {
	t.Parallel()

	where, _, root := objectFixture(t)

	testCases := []struct {
		name   string
		member string
		field  string
		want   entities.DeclKey
	}{
		{
			name:   "a parsed func",
			member: "Own",
			want:   testkit.DeclKey("b/b.go", entities.KindFunc, "Own"),
		},
		{
			name:   "a parsed type",
			member: "OwnType",
			want:   testkit.DeclKey("b/b.go", entities.KindType, "OwnType"),
		},
		{
			name:   "a method of an interface literal inside a type",
			member: "S",
			field:  "F",
			want:   testkit.DeclKey("b/b.go", entities.KindType, "S"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			obj := root.Types.Scope().Lookup(testCase.member)
			if testCase.field != "" {
				obj = anonymousMethod(t, root.Types, testCase.member, testCase.field)
			}

			got, found := where.ObjectAt(root.Fset, obj)
			if !found || got != testCase.want {
				t.Fatalf("ObjectAt() = %v, %v, want %v", got, found, testCase.want)
			}

			again, againFound := where.DeclAt(root.Fset, obj.Pos())
			if again != got || againFound != found {
				t.Fatalf("DeclAt() = %v, %v, want ObjectAt's %v", again, againFound, got)
			}
		})
	}
}

func TestPositionsObjectAtConcurrent(t *testing.T) {
	t.Parallel()

	where, _, root := objectFixture(t)

	testCases := []struct {
		name    string
		obj     types.Object
		want    entities.DeclKey
		workers int
	}{
		{
			name:    "an export-data func",
			obj:     root.Types.Imports()[0].Scope().Lookup("F"),
			want:    testkit.DeclKey("a/a.go", entities.KindFunc, "F"),
			workers: 8,
		},
		{
			name:    "a parsed func",
			obj:     root.Types.Scope().Lookup("Own"),
			want:    testkit.DeclKey("b/b.go", entities.KindFunc, "Own"),
			workers: 8,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			start := make(chan struct{})

			var group sync.WaitGroup

			for range testCase.workers {
				group.Go(func() {
					<-start

					got, found := where.ObjectAt(root.Fset, testCase.obj)
					if !found || got != testCase.want {
						t.Errorf("ObjectAt() = %v, %v, want %v", got, found, testCase.want)
					}
				})
			}

			close(start)
			group.Wait()
		})
	}
}

func BenchmarkPositionsObjectAt(b *testing.B) {
	testCases := []struct {
		name     string
		isParsed bool
		decls    int
	}{
		{name: "export data/0 decls", isParsed: false, decls: 0},
		{name: "export data/10 decls", isParsed: false, decls: 10},
		{name: "export data/100 decls", isParsed: false, decls: 100},
		{name: "export data/1000 decls", isParsed: false, decls: 1000},
		{name: "parsed/0 decls", isParsed: true, decls: 0},
		{name: "parsed/10 decls", isParsed: true, decls: 10},
		{name: "parsed/100 decls", isParsed: true, decls: 100},
		{name: "parsed/1000 decls", isParsed: true, decls: 1000},
	}

	for _, testCase := range testCases {
		b.Run(testCase.name, func(b *testing.B) {
			const width = 10

			root := b.TempDir()
			fset := token.NewFileSet()
			file := fset.AddFile(
				filepath.Join(root, "a", "a.go"), -1, (testCase.decls+1)*width,
			)
			pkg := types.NewPackage("example.test/a", "a")
			sig := types.NewSignatureType(nil, nil, nil, nil, nil, false)
			fn := types.NewFunc(file.Pos(testCase.decls*width+5), pkg, "F", sig)
			pkg.Scope().Insert(fn)

			decls := make([]entities.Decl, 0, testCase.decls+1)
			for index := range testCase.decls {
				decls = append(decls, entities.Decl{
					Key:  testkit.DeclKey("a/a.go", entities.KindFunc, "D"+strconv.Itoa(index)),
					Span: entities.Span{Start: index * width, End: (index + 1) * width},
				})
			}

			lastStart := testCase.decls * width
			decls = append(decls, entities.Decl{
				Key:  testkit.DeclKey("a/a.go", entities.KindFunc, "F"),
				Span: entities.Span{Start: lastStart, End: lastStart + width},
			})

			where := NewPositions(root, NewDeclLocator(decls))
			if testCase.isParsed {
				where = where.Parsing([]*packages.Package{{
					Fset: fset, Syntax: []*ast.File{{FileStart: file.Pos(0)}},
				}})
			}

			want := testkit.DeclKey("a/a.go", entities.KindFunc, "F")

			got, found := where.ObjectAt(fset, fn)
			if !found || got != want {
				b.Fatalf("ObjectAt() = %v, %v, want %v", got, found, want)
			}

			for b.Loop() {
				got, found = where.ObjectAt(fset, fn)
			}

			if !found || got != want {
				b.Fatalf("ObjectAt() after the loop = %v, %v, want %v", got, found, want)
			}
		})
	}
}
