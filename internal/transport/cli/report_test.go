package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
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

// TestInTestFileIsWiderThanIsTest pins the distinction that a real rewrite
// needed and could not get: a stub method inside a test file is not a test
// function. It has no subject to follow and no external reader worth
// following, and judging it by one moves it away from the tests that use it.
func TestInTestFileIsWiderThanIsTest(t *testing.T) {
	t.Parallel()

	suite := testFn("a/lib_test.go", "TestDo", "a")
	stub := entities.Decl{
		Key:  entities.DeclKey{File: "a/lib_test.go", Kind: entities.KindMethod, Name: "fake.Recv"},
		Unit: "a",
	}
	source := fn("a/lib.go", "Do", "a")

	graph := entities.NewGraph([]entities.Decl{suite, stub, source}, nil, nil)
	births := indexing.Births{At: map[entities.DeclKey]entities.Position{
		suite.Key: 1, stub.Key: 1, source.Key: 1,
	}}

	byKey := map[string]reportLine{}
	for _, line := range reportLines(graph, births) {
		byKey[line.Decl] = line
	}

	testCases := []struct {
		name           string
		key            entities.DeclKey
		wantIsTest     bool
		wantInTestFile bool
	}{
		{"a test function is both", suite.Key, true, true},
		{"a stub method lives in a test file but is not a test", stub.Key, false, true},
		{"ordinary source is neither", source.Key, false, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			line := byKey[testCase.key.String()]
			if line.IsTest != testCase.wantIsTest || line.InTestFile != testCase.wantInTestFile {
				t.Fatalf(
					"is_test=%v in_test_file=%v, want %v and %v",
					line.IsTest, line.InTestFile, testCase.wantIsTest, testCase.wantInTestFile,
				)
			}
		})
	}
}

// TestInInterfaceMarksTheWholeMethodSet pins a field that changes what fanout
// means. A method nobody references may still be required: removing it stops
// its type from satisfying an interface, and the compiler complains at a
// conversion the file itself does not mention. Zero readers there is the
// opposite of safe to move.
func TestInInterfaceMarksTheWholeMethodSet(t *testing.T) {
	t.Parallel()

	port := entities.Decl{
		Key:         entities.DeclKey{File: "a/port.go", Kind: entities.KindType, Name: "Reader"},
		Unit:        "a",
		IsInterface: true,
	}
	// The type is declared in one file and its method in another, which is why
	// the lookup keys on the unit rather than the file.
	impl := entities.Decl{
		Key:  entities.DeclKey{File: "b/repo.go", Kind: entities.KindType, Name: "Repo"},
		Unit: "b",
	}
	method := entities.Decl{
		Key:  entities.DeclKey{File: "b/read.go", Kind: entities.KindMethod, Name: "Repo.Read"},
		Unit: "b",
	}
	loose := entities.Decl{
		Key:  entities.DeclKey{File: "b/util.go", Kind: entities.KindFunc, Name: "Helper"},
		Unit: "b",
	}

	graph := entities.NewGraph(
		[]entities.Decl{port, impl, method, loose},
		[]entities.Edge{
			{Dependent: impl.Key, Provider: port.Key, Reason: entities.ReasonImplements},
		},
		nil,
	)

	byKey := map[string]reportLine{}
	for _, line := range reportLines(graph, indexing.Births{}) {
		byKey[line.Decl] = line
	}

	testCases := []struct {
		name string
		key  entities.DeclKey
		want bool
	}{
		{"the satisfying type", impl.Key, true},
		{"a method of it, declared in another file of the same unit", method.Key, true},
		{"a loose function of the same unit", loose.Key, false},
		{"the interface itself is not bound by anyone else's method set", port.Key, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := byKey[testCase.key.String()].InInterface; got != testCase.want {
				t.Fatalf("in_interface = %v, want %v", got, testCase.want)
			}
		})
	}
}

// reportGraph and reportStart give runReport a fake pipeline whose Graph and
// Births return controllable, deterministic data.
func reportStart(
	ctrl *gomock.Controller,
	graphErr, birthsErr error,
) starter {
	pl := NewMockpipeline(ctrl)
	pl.EXPECT().Close(gomock.Any()).Return(nil)

	if graphErr != nil {
		pl.EXPECT().Graph(gomock.Any()).Return(indexing.Result{}, graphErr)

		return func(context.Context, bootstrap.Config) (pipeline, error) { return pl, nil }
	}

	subject := fn("lead/create.go", "Create", "lead")
	graph := entities.NewGraph([]entities.Decl{subject}, nil, nil)
	pl.EXPECT().Graph(gomock.Any()).Return(indexing.Result{Graph: graph}, nil)

	if birthsErr != nil {
		pl.EXPECT().Births(gomock.Any()).Return(indexing.Births{}, birthsErr)
	} else {
		pl.EXPECT().Births(gomock.Any()).Return(indexing.Births{
			At: map[entities.DeclKey]entities.Position{subject.Key: 3},
		}, nil)
	}

	return func(context.Context, bootstrap.Config) (pipeline, error) { return pl, nil }
}

func TestRunReport(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")

	t.Run("the graph stage failing fails the command", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start := reportStart(ctrl, errBoom, nil)

		var out bytes.Buffer

		err := runReport(context.Background(), bootstrap.Config{}, &out, start)
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want it to wrap %v", err, errBoom)
		}
	})

	t.Run("the births stage failing fails the command", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start := reportStart(ctrl, nil, errBoom)

		var out bytes.Buffer

		err := runReport(context.Background(), bootstrap.Config{}, &out, start)
		if !errors.Is(err, errBoom) {
			t.Fatalf("err = %v, want it to wrap %v", err, errBoom)
		}
	})

	t.Run("a write failure surfaces instead of being swallowed", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start := reportStart(ctrl, nil, nil)

		errWrite := errors.New("disk full")

		err := runReport(context.Background(), bootstrap.Config{}, failWriter{err: errWrite}, start)
		if !errors.Is(err, errWrite) {
			t.Fatalf("err = %v, want it to wrap %v", err, errWrite)
		}
	})

	t.Run("success emits one decoded, well-shaped line per declaration", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		start := reportStart(ctrl, nil, nil)

		var out bytes.Buffer

		if err := runReport(context.Background(), bootstrap.Config{}, &out, start); err != nil {
			t.Fatalf("runReport() = %v", err)
		}

		var got reportLine
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}

		wantDecl := fn("lead/create.go", "Create", "lead").Key.String()

		if got.Decl != wantDecl || got.BornAt != 3 {
			t.Fatalf("got = %+v", got)
		}
	})
}
