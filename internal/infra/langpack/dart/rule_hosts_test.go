package dart

import (
	"context"
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestHostsRule(t *testing.T) {
	t.Parallel()

	edges, err := hostsRule{}.Edges(context.Background(), monorepo.space(), declareAll(t, monorepo))
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	got := map[string]string{}
	for _, edge := range edges {
		got[edge.Dependent.File+" "+string(edge.Reason)] = edge.Provider.File
	}

	want := map[string]string{
		"packages/core/lib/core.dart hosts":      "packages/core/pubspec.yaml",
		"packages/core/lib/src/a.dart hosts":     "packages/core/pubspec.yaml",
		"packages/core/lib/src/b/b.dart hosts":   "packages/core/pubspec.yaml",
		"packages/core/test/a_test.dart hosts":   "packages/core/pubspec.yaml",
		"web/src/routes/+page.server.dart hosts": "web/pubspec.yaml",
		"packages/core/pubspec.yaml workspace":   "pubspec.yaml",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %v, want %v", got, want)
	}
}

func TestHostsRuleReadFailure(t *testing.T) {
	t.Parallel()

	failing := newWorkspace([]string{"pubspec.yaml"}, fakeTree{}.Read)
	decls := []entities.Decl{{Key: key("pubspec.yaml", entities.KindOther, fileDeclName)}}

	if _, err := (hostsRule{}).Edges(context.Background(), failing, decls); err == nil {
		t.Fatal("Edges() error = nil, want the read failure")
	}
}
