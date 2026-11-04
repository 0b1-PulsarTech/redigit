package emitting

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()

	var logged bytes.Buffer

	previous := slog.Default()

	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &logged
}

func observeRows(t *testing.T, rows []Commit) string {
	t.Helper()

	logged := captureWarnings(t)
	latest := held{}

	for _, row := range rows {
		latest.observe(row)
	}

	return logged.String()
}

func TestHeldObserveWarnsOnARepeatedRow(t *testing.T) {
	testCases := []struct {
		name      string
		rows      []Commit
		wantParts []string
	}{
		{
			name: "a row repeating the latest handle names position and keys",
			rows: []Commit{
				{
					Position: 0,
					Blobs:    []Blob{{Path: "a.go", Handle: "one"}},
					Chunks:   []Chunk{{Key: key("a.go", "KeyA")}},
				},
				{
					Position: 3,
					Blobs:    []Blob{{Path: "a.go", Handle: "one"}},
					Chunks:   []Chunk{{Key: key("a.go", "KeyA")}, {Key: key("a.go", "KeyB")}},
				},
			},
			wantParts: []string{"position=3", "a.go:func:KeyA", "a.go:func:KeyB"},
		},
		{
			name: "a row compares with the latest earlier row and not the first",
			rows: []Commit{
				{Position: 0, Blobs: []Blob{{Path: "a.go", Handle: "one"}}},
				{Position: 1, Blobs: []Blob{{Path: "a.go", Handle: "two"}}},
				{Position: 2, Blobs: []Blob{{Path: "a.go", Handle: "two"}}},
			},
			wantParts: []string{"position=2"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := observeRows(t, testCase.rows)

			for _, want := range testCase.wantParts {
				if !strings.Contains(got, want) {
					t.Fatalf("log %q lacks %q", got, want)
				}
			}

			if count := strings.Count(got, "level=WARN"); count != 1 {
				t.Fatalf("warned %d times, want 1", count)
			}
		})
	}
}

func TestHeldObserveStaysSilent(t *testing.T) {
	both := []Blob{{Path: "a.go", Handle: "one"}, {Path: "b.go", Handle: "one"}}

	testCases := []struct {
		name string
		rows []Commit
	}{
		{
			name: "a row that changes a file",
			rows: []Commit{
				{Position: 0, Blobs: []Blob{{Path: "a.go", Handle: "one"}}},
				{Position: 1, Blobs: []Blob{{Path: "a.go", Handle: "two"}}},
			},
		},
		{
			name: "a first-seen path",
			rows: []Commit{
				{Position: 0, Blobs: []Blob{{Path: "a.go", Handle: "one"}}},
				{Position: 1, Blobs: []Blob{{Path: "b.go", Handle: "one"}}},
			},
		},
		{
			name: "one changed blob among unchanged ones",
			rows: []Commit{
				{Position: 0, Blobs: both},
				{
					Position: 1,
					Blobs:    []Blob{{Path: "a.go", Handle: "one"}, {Path: "b.go", Handle: "two"}},
				},
			},
		},
		{
			name: "a changed first blob with unchanged later ones",
			rows: []Commit{
				{Position: 0, Blobs: both},
				{
					Position: 1,
					Blobs:    []Blob{{Path: "a.go", Handle: "two"}, {Path: "b.go", Handle: "one"}},
				},
			},
		},
		{
			name: "a return to the first handle after a change",
			rows: []Commit{
				{Position: 0, Blobs: []Blob{{Path: "a.go", Handle: "one"}}},
				{Position: 1, Blobs: []Blob{{Path: "a.go", Handle: "two"}}},
				{Position: 2, Blobs: []Blob{{Path: "a.go", Handle: "one"}}},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := observeRows(t, testCase.rows); got != "" {
				t.Fatalf("warned %q, want nothing", got)
			}
		})
	}
}
