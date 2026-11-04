package anchorprobe

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func checkBy(
	path string,
	original, edited verifying.Outcome,
	failure error,
	calls *int,
) func(string, verifying.Unit) (verifying.Outcome, error) {
	return func(string, verifying.Unit) (verifying.Outcome, error) {
		*calls++

		content, err := os.ReadFile(path)
		if err != nil {
			return verifying.Outcome{}, fmt.Errorf("read: %w", err)
		}

		if string(content) == "A last\n" {
			return original, failure
		}

		return edited, failure
	}
}

type verdictCase struct {
	name          string
	original      verifying.Outcome
	edited        verifying.Outcome
	failure       error
	wantBuilt     bool
	wantUnchecked bool
	wantDetail    string
}

func TestBuildsVerdicts(t *testing.T) {
	t.Parallel()

	pass := verifying.Outcome{Step: "vet"}
	buildFails := verifying.Outcome{IsFailed: true, Step: "build", Tail: "# p\na.go:1:1: no Z"}
	timedOut := verifying.Outcome{
		IsFailed: true, IsTimeout: true, Step: "build", Tail: "\ntimed out after 5m0s",
	}
	looksTimedOut := verifying.Outcome{
		IsFailed: true, Step: "build", Tail: "a.go:3:1: error: timed out after 5s",
	}

	testCases := []verdictCase{
		{
			name: "a unit that builds before and after", original: pass, edited: pass,
			wantBuilt: true,
		},
		{
			name: "a failure the edit brings is a verdict", original: pass, edited: buildFails,
			wantDetail: ".: build: a.go:1:1: no Z",
		},
		{
			name: "a timeout while checking the edit", original: pass, edited: timedOut,
			wantUnchecked: true, wantDetail: "timed out",
		},
		{
			name: "a timeout on the unmodified tree", original: timedOut, edited: timedOut,
			wantUnchecked: true, wantDetail: "timed out",
		},
		{
			name:     "a compile error that only says timed out after is a verdict",
			original: pass, edited: looksTimedOut, wantDetail: "timed out after 5s",
		},
		{
			name: "a builder that cannot run", original: pass, edited: pass,
			failure: errors.New("no toolchain"), wantUnchecked: true,
			wantDetail: ".: no toolchain",
		},
	}

	assertVerdicts(t, testCases)
}

func TestBuildsVerdictsOfAUnitRedBeforeAnyEdit(t *testing.T) {
	t.Parallel()

	pass := verifying.Outcome{Step: "vet"}
	vetFails := verifying.Outcome{IsFailed: true, Step: "vet", Tail: "# p\nl.go:4:2: bad verb %d"}
	redMore := verifying.Outcome{
		IsFailed: true, Step: "vet", Tail: vetFails.Tail + "\na.go:1:1: no Z",
	}
	noisyBefore := verifying.Outcome{
		IsFailed: true, Step: "build", Tail: "/tmp/go-build123/b001/x.go: boom \n",
	}
	noisyAfter := verifying.Outcome{
		IsFailed: true, Step: "build", Tail: "/tmp/go-build456/b001/x.go: boom",
	}
	shiftedBefore := verifying.Outcome{
		IsFailed: true, Step: "build", Tail: "# p\n./a.go:10:23: undefined: G",
	}
	shiftedMoved := verifying.Outcome{
		IsFailed: true, Step: "build", Tail: "# p\n./a.go:7:23: undefined: G",
	}
	redTwice := verifying.Outcome{
		IsFailed: true, Step: "vet", Tail: redMore.Tail + "\na.go:2:1: no Y",
	}
	vetTimedOut := verifying.Outcome{
		IsFailed: true, IsTimeout: true, Step: "vet", Tail: vetFails.Tail + "\ntimed out",
	}
	newHeader := verifying.Outcome{
		IsFailed: true, Step: "build",
		Tail: shiftedBefore.Tail + "\n# example.com/m/b\n./b.go:2:3: undefined: Z",
	}
	shiftedAfter := verifying.Outcome{
		IsFailed: true, Step: "build", Tail: shiftedMoved.Tail + "\n./a.go:9:1: undefined: Z",
	}

	testCases := []verdictCase{
		{
			name:     "a failure that predates every edit gives no verdict",
			original: vetFails, edited: vetFails, wantUnchecked: true,
			wantDetail: ".: fails before any edit: .: vet: l.go:4:2: bad verb %d",
		},
		{
			name:     "a red unit the edit makes redder is a verdict",
			original: vetFails, edited: redMore, wantDetail: ".: vet: a.go:1:1: no Z",
		},
		{
			name:     "a red unit the edit makes green gives no verdict",
			original: vetFails, edited: pass, wantUnchecked: true,
			wantDetail: "fails before any edit",
		},
		{
			name:     "a red unit whose output differs only by the build dir gives no verdict",
			original: noisyBefore, edited: noisyAfter, wantUnchecked: true,
			wantDetail: "fails before any edit",
		},
		{
			name:     "a red unit whose diagnostic only moved lines gives no verdict",
			original: shiftedBefore, edited: shiftedMoved, wantUnchecked: true,
			wantDetail: "fails before any edit",
		},
		{
			name:     "a red unit with a moved diagnostic and a new one is a verdict",
			original: shiftedBefore, edited: shiftedAfter, wantDetail: "a.go:9:1: undefined: Z",
		},
		{
			name:     "a new package header is not the reason",
			original: shiftedBefore, edited: newHeader, wantDetail: "b.go:2:3: undefined: Z",
		},
		{
			name: "the first fresh diagnostic is the reason", original: vetFails, edited: redTwice,
			wantDetail: ".: vet: a.go:1:1: no Z",
		},
		{
			name: "a red unit whose edit times out gives no verdict", original: vetFails,
			edited: vetTimedOut, wantUnchecked: true, wantDetail: "fails before any edit",
		},
	}

	assertVerdicts(t, testCases)
}

func assertVerdicts(t *testing.T, testCases []verdictCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := writeFile(t, dir, "A last\n")

			var calls int

			probe := newProbe(t, wiring{
				dir:   dir,
				check: checkBy(path, testCase.original, testCase.edited, testCase.failure, &calls),
			})

			verdict, err := probe.Builds(t.Context(), []anchoring.Edit{{
				File: "a.go", Span: entities.Span{End: 6}, Text: "A one",
				Keep: []entities.DeclKey{{File: "a.go", Kind: entities.KindFunc, Name: "A"}},
			}})
			if err != nil {
				t.Fatalf("Builds() = %v", err)
			}

			isRight := verdict.IsBuilt == testCase.wantBuilt &&
				verdict.IsUnchecked == testCase.wantUnchecked
			if !isRight || !strings.Contains(verdict.Detail, testCase.wantDetail) {
				t.Fatalf("Builds() = %+v, want %+v", verdict, testCase)
			}
		})
	}
}

func TestBuildsChecksTheUnmodifiedTreeOncePerUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		trials    int
		wantCalls int
	}{
		{name: "two trials of one unit share one baseline", trials: 2, wantCalls: 3},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := writeFile(t, dir, "A last\n")

			var calls int

			pass := verifying.Outcome{Step: "vet"}
			probe := newProbe(t, wiring{dir: dir, check: checkBy(path, pass, pass, nil, &calls)})

			for range testCase.trials {
				if _, err := probe.Builds(t.Context(), []anchoring.Edit{{
					File: "a.go", Span: entities.Span{End: 6}, Text: "A one",
					Keep: []entities.DeclKey{{File: "a.go", Kind: entities.KindFunc, Name: "A"}},
				}}); err != nil {
					t.Fatalf("Builds() = %v", err)
				}
			}

			if calls != testCase.wantCalls {
				t.Fatalf("builds = %d, want one baseline and two trials", calls)
			}
		})
	}
}

func TestDiagnostic(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		tail string
		want string
	}{
		{
			name: "a header then the compiler line",
			tail: "# pkg\nfile.go:3:1: boom\nmore", want: "file.go:3:1: boom",
		},
		{
			name: "blank lines are skipped",
			tail: "\n\n  # pkg\n\n  x.go:1: bad  \n", want: "x.go:1: bad",
		},
		{name: "a header alone stays as the only line", tail: "# pkg\n", want: "# pkg"},
		{name: "no header", tail: "undefined: Z", want: "undefined: Z"},
		{name: "nothing"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := diagnostic(testCase.tail); got != testCase.want {
				t.Fatalf("diagnostic() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestBuildsDoesNotRecheckAUnitThatCouldNotBeChecked(t *testing.T) {
	t.Parallel()

	pass := verifying.Outcome{Step: "vet"}
	timedOut := verifying.Outcome{IsFailed: true, IsTimeout: true, Step: "vet", Tail: "timed out"}

	testCases := []struct {
		name     string
		original verifying.Outcome
		failure  error
	}{
		{name: "a unit that timed out before any edit", original: timedOut},
		{
			name: "a unit whose builder cannot run", original: pass,
			failure: errors.New("no toolchain"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := writeFile(t, dir, "A last\n")

			var calls int

			probe := newProbe(t, wiring{
				dir:   dir,
				check: checkBy(path, testCase.original, pass, testCase.failure, &calls),
			})

			verdict, err := probe.Builds(t.Context(), []anchoring.Edit{{
				File: "a.go", Span: entities.Span{End: 6}, Text: "A one",
				Keep: []entities.DeclKey{{File: "a.go", Kind: entities.KindFunc, Name: "A"}},
			}})
			if err != nil || !verdict.IsUnchecked || calls != 1 {
				t.Fatalf("Builds() = %+v, %v after %d checks, want 1", verdict, err, calls)
			}
		})
	}
}

func TestNewLines(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		before string
		after  string
		want   []string
	}{
		{
			name:   "the checkout path reads as the tree",
			before: "/w/a.go:1:1: boom",
			after:  "/w/a.go:2:1: boom\n/w/b.go:1:1: bad",
			want:   []string{"<tree>/b.go:1:1: bad"},
		},
		{
			name:   "a line without a column that only moved is not a difference",
			before: "/w/a.go:5: boom",
			after:  "/w/a.go:9: boom",
		},
		{
			name:   "a build directory is not a difference",
			before: "/tmp/go-build1/x.go: boom",
			after:  "/tmp/go-build2/x.go: boom",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := newLines(testCase.before, testCase.after, "/w")
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("newLines() = %q, want %q", got, testCase.want)
			}
		})
	}
}
