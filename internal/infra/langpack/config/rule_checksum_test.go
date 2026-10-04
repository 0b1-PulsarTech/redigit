package config

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// A require born before the go.sum holding its hashes stops the go command at
// "missing go.sum entry", on every tree until the file arrives.
func TestChecksumRuleMakesARequireWaitForTheSumBesideIt(t *testing.T) {
	t.Parallel()

	appRequire := piece("apps/lead/go.mod", "require example.com/a", entities.Span{})
	appSum := piece("apps/lead/go.sum", "<file>", entities.Span{})
	libSum := piece("libs/lib/go.sum", "<file>", entities.Span{})
	replace := piece("apps/lead/go.mod", "replace example.com/a", entities.Span{})

	edges, err := (checksumRule{}).Edges(
		t.Context(), entities.Snapshot{}, []entities.Decl{appRequire, appSum, libSum, replace},
	)
	if err != nil {
		t.Fatalf("Edges() = %v", err)
	}

	if !edgeBetween(edges, appRequire.Key, appSum.Key) {
		t.Error("the require does not wait for the go.sum beside its go.mod")
	}

	if edgeBetween(edges, appRequire.Key, libSum.Key) {
		t.Error("the require waits for another module's go.sum")
	}

	if edgeBetween(edges, replace.Key, appSum.Key) {
		t.Error("a replace line waits for a go.sum it needs no hash from")
	}
}
