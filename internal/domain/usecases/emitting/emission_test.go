package emitting

import "testing"

func TestCommitIsEmpty(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		commit Commit
		want   bool
	}{
		{"no blobs is empty", Commit{}, true},
		{"a blob makes it non-empty", Commit{Blobs: []Blob{{Path: "a.go"}}}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.commit.IsEmpty(); got != testCase.want {
				t.Fatalf("IsEmpty() = %v, want %v", got, testCase.want)
			}
		})
	}
}
