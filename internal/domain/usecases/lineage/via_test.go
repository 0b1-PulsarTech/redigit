package lineage

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestRenamesFinal(t *testing.T) {
	t.Parallel()

	oldKey := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "Old"}
	midKey := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "Mid"}
	newKey := entities.DeclKey{File: "b.go", Kind: entities.KindFunc, Name: "Mid"}
	lastKey := entities.DeclKey{File: "b.go", Kind: entities.KindFunc, Name: "Last"}
	stray := entities.DeclKey{File: "z.go"}

	renames := Result{Births: map[entities.DeclKey]Birth{
		lastKey: {At: 1, Via: []Rename{
			{At: 5, From: oldKey, To: midKey},
			{At: 8, From: midKey, To: newKey},
			{At: 12, From: newKey, To: lastKey},
		}},
	}}.Renames()

	testCases := []struct {
		name  string
		key   entities.DeclKey
		after entities.Position
		want  entities.DeclKey
	}{
		{name: "a key nothing renamed", key: stray, after: 0, want: stray},
		{name: "every rename follows an early position", key: oldKey, after: 2, want: lastKey},
		{
			name: "a rename at the anchor itself is already applied",
			key:  oldKey, after: 5, want: oldKey,
		},
		{name: "the chain resumes after the first rename", key: oldKey, after: 4, want: lastKey},
		{name: "a middle key follows only the later renames", key: midKey, after: 5, want: lastKey},
		{name: "after the last rename the key stays", key: newKey, after: 12, want: newKey},
		{name: "a file move followed by a name change", key: midKey, after: 7, want: lastKey},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := renames.Final(testCase.key, testCase.after); got != testCase.want {
				t.Fatalf("Final(%v, %d) = %v, want %v",
					testCase.key, testCase.after, got, testCase.want)
			}
		})
	}
}
