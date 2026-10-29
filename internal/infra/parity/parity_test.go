package parity

import (
	"os"
	"path/filepath"
	"testing"
)

func parityPath(tb testing.TB, name string) string {
	tb.Helper()

	dir := os.Getenv("REDIGIT_PARITY_DIR")
	if dir == "" {
		tb.Skip("REDIGIT_PARITY_DIR is not set; real fixtures are unavailable")
	}

	return filepath.Join(dir, name)
}

func loadReal(tb testing.TB, name string) []Row {
	tb.Helper()

	rows, err := LoadEmit(parityPath(tb, name))
	if err != nil {
		tb.Fatalf("load %s: %v", name, err)
	}

	return rows
}

func TestRealFixtures(t *testing.T) {
	parityPath(t, "")

	passZero := loadReal(t, "r21k_x0.jsonl")
	passTwo := loadReal(t, "r21k_x.jsonl")
	hand := loadReal(t, "redigit-hand_hand12.jsonl")
	passZeroFmt := loadReal(t, "r21k_x0-fmt.jsonl")

	hard, err := LoadHardEdges(parityPath(t, "r9_plan.jsonl.edges"))
	if err != nil {
		t.Fatalf("load edges: %v", err)
	}

	pairs := []struct {
		name        string
		left, right []Row
	}{
		{"x0 vs x", passZero, passTwo},
		{"hand12 vs x0", hand, passZero},
		{"x0 vs x0-fmt", passZero, passZeroFmt},
	}
	for _, pair := range pairs {
		rowSet, setErr := RowSetEquality(pair.left, pair.right)
		if setErr != nil {
			t.Fatalf("%s: row set: %v", pair.name, setErr)
		}

		tau := KendallTau(pair.left, pair.right)
		fraction, fractionDefined := rowSet.Fraction()
		t.Logf(
			"%s: row-set %d/%d = %.4f defined %v (differing positions %d), kendall tau %.4f defined %v over %d common keys",
			pair.name,
			rowSet.Measure.Value,
			rowSet.Measure.Total,
			fraction,
			fractionDefined,
			len(rowSet.Differs),
			tau.Tau,
			tau.Defined,
			tau.Common,
		)

		if pair.name == "x0 vs x0-fmt" && (!fractionDefined || fraction != 1) {
			t.Errorf("formatting moved chunks: differing positions %v", rowSet.Differs)
		}
	}

	distinct := make(map[edgePair]struct{}, len(hard))
	for _, edge := range hard {
		distinct[edgePair{dependent: edge.Dependent, provider: edge.Provider}] = struct{}{}
	}

	t.Logf("hard edges loaded: %d lines, %d distinct pairs", len(hard), len(distinct))

	for _, plan := range []struct {
		name string
		rows []Row
	}{{"hand12", hand}, {"x0", passZero}, {"x", passTwo}} {
		result := Violations(plan.rows, hard)
		t.Logf(
			"violations %s: %d of %d checked hard edges; first example %v",
			plan.name,
			result.Measure.Value,
			result.Measure.Total,
			result.Examples[:min(1, len(result.Examples))],
		)
	}
}

func BenchmarkLoadEmit(b *testing.B) {
	path := parityPath(b, "r21k_x0.jsonl")

	b.ReportAllocs()

	for b.Loop() {
		if _, err := LoadEmit(path); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKendallTau(b *testing.B) {
	passZero, passTwo := loadReal(b, "r21k_x0.jsonl"), loadReal(b, "r21k_x.jsonl")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		KendallTau(passZero, passTwo)
	}
}

func BenchmarkRowSetEquality(b *testing.B) {
	passZero, passTwo := loadReal(b, "r21k_x0.jsonl"), loadReal(b, "r21k_x.jsonl")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := RowSetEquality(passZero, passTwo); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadHardEdges(b *testing.B) {
	path := parityPath(b, "r9_plan.jsonl.edges")

	b.ReportAllocs()

	for b.Loop() {
		if _, err := LoadHardEdges(path); err != nil {
			b.Fatal(err)
		}
	}
}
