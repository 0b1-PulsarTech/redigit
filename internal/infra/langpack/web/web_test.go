package web

import (
	"context"
	"errors"
	"maps"
	"path"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var errMissing = errors.New("missing")

type fakeTree map[string]string

func (t fakeTree) Read(_ context.Context, file string) ([]byte, error) {
	source, found := t[file]
	if !found {
		return nil, errMissing
	}

	return []byte(source), nil
}

func (t fakeTree) files() []string { return slices.Sorted(maps.Keys(t)) }

func (t fakeTree) space() *workspace { return newWorkspace(t.files(), t.Read) }

// app is a small SvelteKit workspace: a web package, a component package it
// imports by name, a virtual module, a type cycle, and Node test scripts.
var app = fakeTree{
	"package.json": `{"name": "root", "workspaces": ["web", "ui"]}`,
	"ui/package.json": `{"name": "ui", "exports": {
		"./*": "./svelte/*", "./format": "./svelte/format.ts"}}`,
	"ui/svelte/format.ts":         "export const x = 1;\n",
	"ui/svelte/atoms/Icon.svelte": "<script lang=\"ts\">import { x } from '../format';</script>\n",
	"web/package.json": `{"name": "web", "imports": {
		"#lib/*": "./src/lib/*", "#test/*": "./test/*"}}`,
	"web/tsconfig.json": `{"compilerOptions": {
		"paths": {"virtual:app": ["./src/virtual-app.d.ts"]}}}`,
	"web/src/virtual-app.d.ts":                   "export const app: string;\n",
	"web/src/lib/util.ts":                        "export * from './more.js';\n",
	"web/src/lib/more.ts":                        "export const more = 1;\n",
	"web/src/lib/state.svelte.ts":                "export const s = 1;\n",
	"web/src/lib/a.ts":                           "import type { B } from './b';\nexport type A = B;\n",
	"web/src/lib/b.ts":                           "import type { A } from './a';\nexport type B = A;\n",
	"web/src/routes/+page.svelte":                routePage,
	"web/src/routes/+page.server.dart":           "Object? load() => null;\n",
	"web/src/routes/conversas/[id]/+page.svelte": "<p>conversa</p>\n",
	"web/tool/client_entry.dart":                 "void main() {}\n",
	"web/test/harness.mjs":                       "export const h = 1;\n",
	"web/test/page.mjs": "import { h } from '#test/harness.mjs';\n" +
		"await fetch(base + '/conversas/abc');\n",
	"web/test/util.test.ts":   "import { more } from '#lib/util';\n",
	"web/test/runner.mjs":     "import { run } from './cases/abre.mjs';\nawait run();\n",
	"web/test/cases/abre.mjs": "export const run = () => goto(`${base}/conversas/${id}`);\n",
	"web/test/bare.mjs":       "import 'node:fs';\n",
}

const routePage = `<script lang="ts">
	import Icon from 'ui/atoms/Icon.svelte';
	import { x } from 'ui/format';
	import { app } from 'virtual:app';
	import { s } from '#lib/state.svelte';
	import { more } from '#lib/util';
</script>
`

var virtual = map[string]string{"virtual:app": "web/tool/client_entry.dart"}

// declareAll reads every script of tree through Syntax and stands each other
// file for whole-file declarations, two for a Dart route module.
func declareAll(t *testing.T, tree fakeTree) []entities.Decl {
	t.Helper()

	syntax := NewSyntax(tree.files(), tree)

	var all []entities.Decl

	for _, file := range tree.files() {
		switch {
		case isScript(file):
			decls, err := syntax.Declarations(context.Background(), file, []byte(tree[file]))
			if err != nil {
				t.Fatalf("Declarations(%s) error = %v", file, err)
			}

			all = append(all, decls...)
		case path.Base(file) == "+page.server.dart":
			all = append(
				all,
				entities.Decl{
					Key:  key(file, entities.KindFunc, "load"),
					Span: entities.Span{Start: 0},
				},
				entities.Decl{
					Key:  key(file, entities.KindValue, "actions"),
					Span: entities.Span{Start: 9},
				},
			)
		default:
			all = append(all, entities.Decl{Key: fileKey(file)})
		}
	}

	return all
}

func key(file string, kind entities.DeclKind, name string) entities.DeclKey {
	return entities.DeclKey{File: file, Kind: kind, Name: name}
}

func fileKey(file string) entities.DeclKey { return key(file, entities.KindOther, fileDeclName) }

// appIndex is one run's index over app, as the rule pack builds it.
func appIndex(t *testing.T) *index {
	t.Helper()

	run := RulePack{virtual: virtual}.index(entities.Snapshot{}, declareAll(t, app))
	run.space = app.space()

	return run
}

// edgesFrom runs one rule and keeps the edges leaving file, as
// "provider reason" strings.
func edgesFrom(t *testing.T, each rule, file string) []string {
	t.Helper()

	edges, err := each.Edges(context.Background(), appIndex(t))
	if err != nil {
		t.Fatalf("%s Edges() error = %v", each.Name(), err)
	}

	var out []string

	for _, edge := range edges {
		if edge.Dependent.File == file {
			out = append(out, edge.Provider.File+":"+edge.Provider.Name+" "+string(edge.Reason))
		}
	}

	slices.Sort(out)

	return out
}
