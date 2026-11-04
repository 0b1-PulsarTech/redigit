package slicing

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
	"github.com/wrapped-owls/redigit/internal/infra/sliceconf"
)

const policySource = `package app

func Reassign() error {
	err := one()
	if err != nil {
		return err
	}

	err = two()
	if err != nil {
		return err
	}

	return nil
}

func Define() error {
	err := one()
	if err != nil {
		return err
	}

	value, err := two()
	if err != nil {
		return err
	}

	use(value)

	return nil
}

func Dangling() {
	err := one()
	if err != nil {
		report(err)
	}

	err = two()
	finish()
}

func OtherName() error {
	problem := one()
	if problem != nil {
		return problem
	}

	problem = two()
	if problem != nil {
		return problem
	}

	return nil
}

func NextReader() error {
	err := one()
	if err != nil {
		return err
	}

	err = two()
	finish()

	if err != nil {
		return err
	}

	return nil
}

func TwoReaders() error {
	err := one()
	if err != nil {
		return err
	}

	err = two()
	if err != nil {
		return err
	}

	report(err)

	return nil
}
`

func policySlice(t *testing.T, name string, policy sliceconf.Config) []entities.Decl {
	t.Helper()

	source := []byte(policySource)

	decls, err := syntax.Declarations(context.Background(), "app/app.go", source)
	if err != nil {
		t.Fatalf("Declarations() error = %v", err)
	}

	for _, decl := range decls {
		if decl.Key.Name != name {
			continue
		}

		sliced, ok, sliceErr := Slice(context.Background(), decl, source, policy)
		if sliceErr != nil || !ok {
			t.Fatalf("Slice(%s) ok = %v, error = %v", name, ok, sliceErr)
		}

		return append(sliced.Members, sliced.Head)
	}

	t.Fatalf("no declaration %q", name)

	return nil
}

type joinCase struct {
	name       string
	function   string
	policy     sliceconf.Config
	left       int
	right      int
	wantJoined bool
}

func TestPolicyJoinsAnErrReassignWithItsCheck(t *testing.T) {
	t.Parallel()

	testCases := []joinCase{
		{
			name:       "a reassign and its check are one atom when errcheck is on",
			function:   "Reassign",
			policy:     sliceconf.Config{Errcheck: true},
			left:       2,
			right:      3,
			wantJoined: true,
		},
		{
			name:       "a reassign and its check stay apart when errcheck is off",
			function:   "Reassign",
			policy:     sliceconf.Config{},
			left:       2,
			right:      3,
			wantJoined: false,
		},
		{
			name:       "the first declaration keeps its own atom apart from the reassign",
			function:   "Reassign",
			policy:     sliceconf.Config{Errcheck: true},
			left:       0,
			right:      2,
			wantJoined: false,
		},
		{
			name:       "a := pair is untouched when errcheck is on",
			function:   "Define",
			policy:     sliceconf.Config{Errcheck: true},
			left:       2,
			right:      3,
			wantJoined: false,
		},
		{
			name:       "a := pair is untouched when errcheck is off",
			function:   "Define",
			policy:     sliceconf.Config{},
			left:       2,
			right:      3,
			wantJoined: false,
		},
		{
			name:       "a reassign nothing checks has nothing to join",
			function:   "Dangling",
			policy:     sliceconf.Config{Errcheck: true},
			left:       2,
			right:      3,
			wantJoined: false,
		},
	}

	joinRows(t, testCases)
}

func TestPolicyJoinsTheNextReaderOfErrOnly(t *testing.T) {
	t.Parallel()

	testCases := []joinCase{
		{
			name:       "a variable not named err is not joined to its check",
			function:   "OtherName",
			policy:     sliceconf.Config{Errcheck: true},
			left:       2,
			right:      3,
			wantJoined: false,
		},
		{
			name:       "a reassign joins the next statement that reads err, not the next one",
			function:   "NextReader",
			policy:     sliceconf.Config{Errcheck: true},
			left:       2,
			right:      4,
			wantJoined: true,
		},
		{
			name:       "a statement between a reassign and its reader stays apart",
			function:   "NextReader",
			policy:     sliceconf.Config{Errcheck: true},
			left:       2,
			right:      3,
			wantJoined: false,
		},
		{
			name:       "a reassign joins its first reader",
			function:   "TwoReaders",
			policy:     sliceconf.Config{Errcheck: true},
			left:       2,
			right:      3,
			wantJoined: true,
		},
		{
			name:       "a reassign does not join a later reader of the same err",
			function:   "TwoReaders",
			policy:     sliceconf.Config{Errcheck: true},
			left:       2,
			right:      4,
			wantJoined: false,
		},
	}

	joinRows(t, testCases)
}

func joinRows(t *testing.T, testCases []joinCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parts := policySlice(t, testCase.function, testCase.policy)
			left, right := parts[testCase.left].Atom, parts[testCase.right].Atom

			if joined := left != "" && left == right; joined != testCase.wantJoined {
				t.Fatalf("atoms = %q and %q, joined = %v, want %v",
					left, right, joined, testCase.wantJoined)
			}
		})
	}
}

func TestPolicyKeepsEveryAtomWithAtLeastTwoParts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		function string
		policy   sliceconf.Config
	}{
		{
			name:     "reassign with errcheck",
			function: "Reassign",
			policy:   sliceconf.Config{Errcheck: true},
		},
		{
			name:     "define with errcheck",
			function: "Define",
			policy:   sliceconf.Config{Errcheck: true},
		},
		{
			name:     "dangling with errcheck",
			function: "Dangling",
			policy:   sliceconf.Config{Errcheck: true},
		},
		{
			name: "reassign without errcheck", function: "Reassign", policy: sliceconf.Config{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			sizes := map[string]int{}
			for _, part := range policySlice(t, testCase.function, testCase.policy) {
				sizes[part.Atom]++
			}

			for atom, size := range sizes {
				if atom != "" && size < 2 {
					t.Fatalf("atom %q holds %d part, want at least 2", atom, size)
				}
			}
		})
	}
}
