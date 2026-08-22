package cli

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

func fn(file, name string, unit entities.UnitID) entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name},
		Unit: unit,
	}
}

func testFn(file, name string, unit entities.UnitID) entities.Decl {
	out := fn(file, name, unit)
	out.IsTest = true

	return out
}

// TestReportCarriesTheTestSubject pins the field a consumer of this report
// needs and could not get: placing a test by its readers moves it away from
// the function it covers, and leaves that function untested at the position it
// is born. The subject is the only anchor a test may be placed by.
func TestReportCarriesTheTestSubject(t *testing.T) {
	t.Parallel()

	subject := fn("lead/create.go", "Create", "lead")
	suite := testFn("lead/create_test.go", "TestCreate", "lead")
	runner := fn("harness/run.go", "Run", "harness")

	graph := entities.NewGraph(
		[]entities.Decl{subject, suite, runner},
		[]entities.Edge{
			// A distant reader of the test, which is exactly what must NOT
			// decide where the test goes.
			{Dependent: runner.Key, Provider: suite.Key, Reason: entities.ReasonReference},
		},
		[]entities.Anchor{{Decl: suite.Key, To: subject.Key, Reason: entities.ReasonTestOf}},
	)

	births := indexing.Births{At: map[entities.DeclKey]entities.Position{
		subject.Key: 10,
		suite.Key:   40,
		runner.Key:  562,
	}}

	byKey := map[string]reportLine{}
	for _, line := range reportLines(graph, births) {
		byKey[line.Decl] = line
	}

	line := byKey[suite.Key.String()]

	if line.Subject == nil {
		t.Fatal("the test row carries no subject")
	}

	testCases := []struct {
		name string
		got  any
		want any
	}{
		{"the subject is the declaration it exercises", line.Subject.Decl, subject.Key.String()},
		{"dated where the subject was born", line.Subject.At, 10},
		{"the gap is measured against the subject", line.SubjectGap, 30},
		{"and the distant reader is still reported, just not as the anchor", line.Gap, 522},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.got != testCase.want {
				t.Fatalf("got %v, want %v", testCase.got, testCase.want)
			}
		})
	}

	if byKey[subject.Key.String()].Subject != nil {
		t.Fatal("a non-test row carries a subject")
	}
}

func TestReportSubjectGapGoesNegativeWhenATestPrecedesIt(t *testing.T) {
	t.Parallel()

	subject := fn("a/lib.go", "Do", "a")
	suite := testFn("a/lib_test.go", "TestDo", "a")

	graph := entities.NewGraph(
		[]entities.Decl{subject, suite},
		nil,
		[]entities.Anchor{{Decl: suite.Key, To: subject.Key, Reason: entities.ReasonTestOf}},
	)

	births := indexing.Births{At: map[entities.DeclKey]entities.Position{
		subject.Key: 50,
		suite.Key:   20,
	}}

	for _, line := range reportLines(graph, births) {
		if line.Decl != suite.Key.String() {
			continue
		}

		// A test born before what it exercises cannot compile, and a positive
		// number here would hide that.
		if line.SubjectGap != -30 {
			t.Fatalf("SubjectGap = %d, want -30", line.SubjectGap)
		}

		return
	}

	t.Fatal("the test row is missing")
}
