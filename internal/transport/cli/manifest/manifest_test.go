package manifest

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
)

type failWriter struct{}

var errBroken = errors.New("broken pipe")

func (failWriter) Write([]byte) (int, error) { return 0, errBroken }

func TestRoundTripKeepsEveryFieldByteForByte(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{
			"a full row with every list filled",
			`{"position":2,"subject":"DRAFT: feat(a): b","shape":"feature","feature":"f","reason":"r",` +
				`"is_wip":true,"chunks":[{"key":"k","span":[1,9],"unit":"decl"}],` +
				`"blobs":[{"path":"a.go","mode":"100644","handle":"h"}],` +
				`"risks":[{"chunk":"c","target":"t","born_at":3}]}` + "\n",
		},
		{
			"a row with empty lists and no optional text",
			`{"position":0,"subject":"s","shape":"x","is_wip":false,"chunks":[],"blobs":[],"risks":[]}` +
				"\n",
		},
		{"no rows at all", ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rows, err := Decode(strings.NewReader(testCase.input))
			if err != nil {
				t.Fatalf("Decode() err = %v", err)
			}

			var out bytes.Buffer
			if err = Encode(&out, rows); err != nil {
				t.Fatalf("Encode() err = %v", err)
			}

			if out.String() != testCase.input {
				t.Fatalf("round trip = %q, want %q", out.String(), testCase.input)
			}
		})
	}
}

func TestDecodeReadsEachField(t *testing.T) {
	t.Parallel()

	line := `{"position":4,"subject":"s","shape":"sh","feature":"f","reason":"r","is_wip":true,` +
		`"chunks":[{"key":"k","span":[1,9],"unit":"u"}],` +
		`"blobs":[{"path":"p","mode":"100755","handle":"h"}],` +
		`"risks":[{"chunk":"c","target":"t","born_at":7}]}`

	rows, err := Decode(strings.NewReader(line))
	if err != nil || len(rows) != 1 {
		t.Fatalf("Decode() = %v, %v", rows, err)
	}

	got := rows[0]
	want := Row{
		Position: 4, Subject: "s", Shape: "sh", Feature: "f", Reason: "r", IsWIP: true,
		Chunks: []Chunk{{Key: "k", Span: [2]int{1, 9}, Unit: "u"}},
		Blobs:  []Blob{{Path: "p", Mode: "100755", Handle: "h"}},
		Risks:  []Risk{{Chunk: "c", Target: "t", BornAt: 7}},
	}

	if got.Position != want.Position || got.Subject != want.Subject || got.Shape != want.Shape ||
		got.Feature != want.Feature || got.Reason != want.Reason || got.IsWIP != want.IsWIP ||
		got.Chunks[0] != want.Chunks[0] || got.Blobs[0] != want.Blobs[0] ||
		got.Risks[0] != want.Risks[0] {
		t.Fatalf("row = %+v, want %+v", got, want)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{"a broken line", "{nope"},
		{"a wrong typed field", `{"position":"one"}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Decode(strings.NewReader(testCase.input))
			if !errors.Is(err, rewriting.ErrManifest) {
				t.Fatalf("Decode() err = %v, want it to wrap ErrManifest", err)
			}
		})
	}
}

func TestEncodeWritesEmptyListsForNilOnes(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := Encode(&out, []Row{{Subject: "s"}}); err != nil {
		t.Fatalf("Encode() err = %v", err)
	}

	want := `{"position":0,"subject":"s","shape":"","is_wip":false,"chunks":[],"blobs":[],"risks":[]}` +
		"\n"
	if out.String() != want {
		t.Fatalf("Encode() = %q, want %q", out.String(), want)
	}
}

func TestEncodeReportsAFailingWriter(t *testing.T) {
	t.Parallel()

	if err := Encode(failWriter{}, []Row{{}}); !errors.Is(err, errBroken) {
		t.Fatalf("Encode() err = %v, want it to wrap %v", err, errBroken)
	}
}

func TestRoundTripKeepsRealEmitOutputByteForByte(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		file string
	}{
		{"three rows copied from an emit run", "testdata/emit.jsonl"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			content, err := os.ReadFile(testCase.file)
			if err != nil {
				t.Fatalf("ReadFile() err = %v", err)
			}

			rows, err := Decode(bytes.NewReader(content))
			if err != nil || len(rows) != 3 {
				t.Fatalf("Decode() = %d rows, err = %v", len(rows), err)
			}

			var out bytes.Buffer
			if err = Encode(&out, rows); err != nil {
				t.Fatalf("Encode() err = %v", err)
			}

			if !bytes.Equal(out.Bytes(), content) {
				t.Fatalf("round trip differs from %s", testCase.file)
			}
		})
	}
}
