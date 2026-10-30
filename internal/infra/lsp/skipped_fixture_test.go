//go:build integration

package lsp

import (
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func skippedModule() map[string]string {
	return map[string]string{
		"testdata/own/own.go":      "package own\n\nfunc F() {}\n",
		"testdata/own/own_test.go": "package own\n\nfunc use() { F() }\n",
		"testdata/own/x_test.go": "package own_test\n\nimport \"example.com/fixture/testdata/own\"\n\n" +
			"func useX() { own.F() }\n",
		"testdata/imp/imp.go": "package imp\n\nfunc H() {}\n",
		"testdata/imp/doc.go": "package imp\n",
		"xmain/cell.go": "package xmain\n\ntype Cell struct{}\n\nfunc (Cell) Do() error { return nil }\n\n" +
			"type Sink interface{ Put() int }\n",
		"testdata/xsrc/src.go": "package xsrc\n\ntype Source interface{ Do() error }\n\n" +
			"type Bin struct{}\n\nfunc (Bin) Put() int { return 0 }\n",
		"tdimp/user.go": "package tdimp\n\nimport \"example.com/fixture/testdata/imp\"\n\n" +
			"func Use() { imp.H() }\n",
	}
}

func skippedCases(dir string) []matchCase {
	ownF := entities.Location{File: filepath.Join(dir, "testdata/own/own.go"), Line: 2, Char: 5}
	inTest := entities.Location{File: "testdata/own/own_test.go", Line: 2, Char: 13}
	external := entities.Location{File: "testdata/own/x_test.go", Line: 4, Char: 18}

	return []matchCase{
		{
			"a function of a testdata package asked cold: its own test file reads it, its external " +
				"test is not loaded yet",
			true,
			ownF,
			[]entities.Location{inTest},
		},
		{
			"a function of an external test of a testdata package, asked after",
			true,
			entities.Location{File: filepath.Join(dir, "testdata/own/x_test.go"), Line: 4, Char: 5},
			nil,
		},
		{
			"the same function once its external test has been asked about",
			true,
			ownF,
			[]entities.Location{inTest, external},
		},
		{
			"a function of a testdata package read by a package of the module that imports it",
			true,
			entities.Location{File: filepath.Join(dir, "testdata/imp/imp.go"), Line: 2, Char: 5},
			[]entities.Location{{File: "tdimp/user.go", Line: 4, Char: 17}},
		},
		{
			"the package clause of a file that declares nothing, in a testdata package a package " +
				"of the module imports",
			true,
			entities.Location{File: filepath.Join(dir, "testdata/imp/doc.go"), Line: 0, Char: 9},
			[]entities.Location{{File: "tdimp/user.go", Line: 2, Char: 7}},
		},
		{
			"a testdata interface met by a type of the main module",
			false,
			entities.Location{File: filepath.Join(dir, "testdata/xsrc/src.go"), Line: 2, Char: 5},
			[]entities.Location{{File: "xmain/cell.go", Line: 2, Char: 5}},
		},
		{
			"a main interface met by a type of testdata",
			false,
			entities.Location{File: filepath.Join(dir, "xmain/cell.go"), Line: 6, Char: 5},
			[]entities.Location{{File: "testdata/xsrc/src.go", Line: 4, Char: 5}},
		},
	}
}
