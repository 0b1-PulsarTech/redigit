package dart

import (
	"context"
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const renderSource = `// header

import 'dart:async';
import 'money.dart';
import 'package:http/http.dart' as http;

class A { Money m; }

class B { void f() => http.get(); }
`

var renderTree = fakeTree{
	"lib/money.dart": "class Money {}\n",
	"lib/use.dart":   renderSource,
}

func TestRender(t *testing.T) {
	t.Parallel()

	const file = "lib/use.dart"

	type testCase struct {
		name   string
		source string
		keep   []entities.DeclKey
		want   string
	}

	testCases := []testCase{
		{
			"an import only a dropped declaration read goes with it",
			renderSource,
			[]entities.DeclKey{key(file, entities.KindType, "A")},
			"// header\n\nimport 'dart:async';\nimport 'money.dart';\n\nclass A { Money m; }\n",
		},
		{
			"an outside import stays while its prefix is read",
			renderSource,
			[]entities.DeclKey{key(file, entities.KindType, "B")},
			"// header\n\nimport 'dart:async';\nimport 'package:http/http.dart' as http;\n\n" +
				"class B { void f() => http.get(); }\n",
		},
		{
			"a kept key the file lacks leaves nothing to write",
			renderSource,
			[]entities.DeclKey{key(file, entities.KindType, "Gone")},
			"",
		},
		{
			"the whole-file declaration writes the file as is",
			"library a;\n",
			[]entities.DeclKey{key(file, entities.KindOther, fileDeclName)},
			"library a;\n",
		},
		{
			"a generated file is written as is",
			"// GENERATED CODE - DO NOT MODIFY BY HAND\n\nint x = 1;\nint y = 2;\n",
			[]entities.DeclKey{key(file, entities.KindValue, "x")},
			"// GENERATED CODE - DO NOT MODIFY BY HAND\n\nint x = 1;\nint y = 2;\n",
		},
		{"keeping nothing writes nothing", renderSource, nil, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			syntax := NewSyntax(renderTree.files(), renderTree)

			got, err := syntax.Render(context.Background(), []byte(tc.source), tc.keep)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			if string(got) != tc.want {
				t.Fatalf("Render() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderReadFailure(t *testing.T) {
	t.Parallel()

	syntax := NewSyntax([]string{"lib/money.dart", "lib/use.dart"}, fakeTree{})

	_, err := syntax.Render(context.Background(), []byte(renderSource), []entities.DeclKey{
		key("lib/use.dart", entities.KindType, "A"),
	})
	if !errors.Is(err, errMissing) {
		t.Fatalf("Render() error = %v, want %v", err, errMissing)
	}
}
