package preimage

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

func entryOf(oldPath, newPath, oldBlob, newBlob string) []batch.Entry {
	return []batch.Entry{{OldPath: oldPath, NewPath: newPath, OldBlob: oldBlob, NewBlob: newBlob}}
}

func changeOf(oldPath, path, oldBlob, blob string) []anchoring.FileChange {
	return []anchoring.FileChange{{Path: path, OldPath: oldPath, Blob: blob, OldBlob: oldBlob}}
}

func TestChangesOf(t *testing.T) {
	t.Parallel()

	zero := strings.Repeat("0", 40)

	testCases := []struct {
		name    string
		entries []batch.Entry
		want    []anchoring.FileChange
	}{
		{
			name:    "a modified file keeps both blobs",
			entries: entryOf("a.go", "a.go", "o1", "n1"),
			want:    changeOf("a.go", "a.go", "o1", "n1"),
		},
		{
			name:    "an added file has no old blob",
			entries: entryOf("b.go", "b.go", zero, "n2"),
			want:    changeOf("b.go", "b.go", "", "n2"),
		},
		{
			name:    "a deleted file has no new blob",
			entries: entryOf("c.go", "c.go", "o3", zero),
			want:    changeOf("c.go", "c.go", "o3", ""),
		},
		{
			name:    "a renamed file keeps both paths",
			entries: entryOf("d.go", "e.go", "o4", "n4"),
			want:    changeOf("d.go", "e.go", "o4", "n4"),
		},
		{name: "no entry is no change", want: []anchoring.FileChange{}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := changesOf(testCase.entries); !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("changesOf() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}
