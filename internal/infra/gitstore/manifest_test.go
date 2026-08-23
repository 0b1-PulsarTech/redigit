package gitstore

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// manifestRoundTripCase is one round-trip fixture: given is what
// EncodeManifest is fed, want is what DecodeManifest must hand back after
// reading it - distinct from given whenever a nil Chunks/Blobs/Risks slice
// must come back as an empty one, never nil.
type manifestRoundTripCase struct {
	name  string
	given []Commit
	want  []Commit
}

func manifestRoundTripCases() []manifestRoundTripCase {
	return []manifestRoundTripCase{
		{"no commits", nil, nil},
		{
			"one feature commit",
			[]Commit{
				{
					Position: 41,
					Shape:    ShapeFeature,
					Feature:  "modules/lead",
					Subject:  "DRAFT: feat(lead): Lead, Create",
					Chunks: []Chunk{
						{Key: "modules/lead/lead.go:type:Lead", Span: [2]int{120, 340}},
					},
					Blobs: []Blob{
						{Path: "modules/lead/lead.go", OID: "3f2a", Mode: ModeFile},
					},
					Risks: []Risk{},
				},
			},
			[]Commit{
				{
					Position: 41,
					Shape:    ShapeFeature,
					Feature:  "modules/lead",
					Subject:  "DRAFT: feat(lead): Lead, Create",
					Chunks: []Chunk{
						{Key: "modules/lead/lead.go:type:Lead", Span: [2]int{120, 340}},
					},
					Blobs: []Blob{
						{Path: "modules/lead/lead.go", OID: "3f2a", Mode: ModeFile},
					},
					Risks: []Risk{},
				},
			},
		},
		{
			"a layer commit with a risk and no oid",
			[]Commit{
				{
					Position: 0,
					Shape:    ShapeLayer,
					Subject:  "DRAFT: chore: scaffold module",
					Chunks:   []Chunk{{Key: "go.mod:other:<file>", Span: [2]int{0, 12}}},
					Blobs:    []Blob{{Path: "go.mod", Mode: ModeFile}},
					Risks: []Risk{
						{Chunk: "a.go:func:Do", Reference: "b.go:type:Repo", BornAt: 5},
					},
				},
			},
			[]Commit{
				{
					Position: 0,
					Shape:    ShapeLayer,
					Subject:  "DRAFT: chore: scaffold module",
					Chunks:   []Chunk{{Key: "go.mod:other:<file>", Span: [2]int{0, 12}}},
					Blobs:    []Blob{{Path: "go.mod", Mode: ModeFile}},
					Risks: []Risk{
						{Chunk: "a.go:func:Do", Reference: "b.go:type:Repo", BornAt: 5},
					},
				},
			},
		},
		{
			// Chunks/Blobs/Risks are left nil on purpose: EncodeManifest must
			// normalize them, so what decodes back is []T{}, never nil.
			"several commits keep their order and nil slices come back empty",
			[]Commit{
				{Position: 0, Shape: ShapeLayer, Subject: "first"},
				{Position: 1, Shape: ShapeFeature, Subject: "second"},
			},
			[]Commit{
				{
					Position: 0, Shape: ShapeLayer, Subject: "first",
					Chunks: []Chunk{}, Blobs: []Blob{}, Risks: []Risk{},
				},
				{
					Position: 1, Shape: ShapeFeature, Subject: "second",
					Chunks: []Chunk{}, Blobs: []Blob{}, Risks: []Risk{},
				},
			},
		},
	}
}

func TestEncodeDecodeManifestRoundTrips(t *testing.T) {
	t.Parallel()

	for _, testCase := range manifestRoundTripCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			if err := EncodeManifest(&buf, testCase.given); err != nil {
				t.Fatalf("EncodeManifest() err = %v", err)
			}

			got, err := DecodeManifest(&buf)
			if err != nil {
				t.Fatalf("DecodeManifest() err = %v", err)
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("round trip = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

func TestEncodeManifestNilSlicesEncodeAsEmptyArrays(t *testing.T) {
	t.Parallel()

	commit := Commit{Position: 0, Shape: ShapeLayer, Subject: "scaffold"}

	var buf bytes.Buffer

	if err := EncodeManifest(&buf, []Commit{commit}); err != nil {
		t.Fatalf("EncodeManifest() err = %v", err)
	}

	line := buf.String()

	for _, want := range []string{`"chunks":[]`, `"blobs":[]`, `"risks":[]`} {
		if !strings.Contains(line, want) {
			t.Fatalf("manifest line %s does not contain %s", line, want)
		}
	}

	for _, forbidden := range []string{`"chunks":null`, `"blobs":null`, `"risks":null`} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("manifest line %s contains %s, want a normalized empty array", line, forbidden)
		}
	}
}

func TestEncodeManifestRefusesInvalidUTF8BeforeWritingAnything(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		commit    Commit
		wantField string
	}{
		{"invalid subject", Commit{Position: 1, Subject: "bad\xff", Risks: []Risk{}}, "subject"},
		{"invalid feature", Commit{Position: 2, Feature: "bad\xff", Risks: []Risk{}}, "feature"},
		{
			"invalid chunk key",
			Commit{Position: 3, Chunks: []Chunk{{Key: "bad\xff"}}, Risks: []Risk{}},
			"chunks[0].key",
		},
		{
			"invalid blob path",
			Commit{Position: 4, Blobs: []Blob{{Path: "bad\xff"}}, Risks: []Risk{}},
			"blobs[0].path",
		},
		{
			"invalid risk chunk",
			Commit{Position: 5, Risks: []Risk{{Chunk: "bad\xff"}}},
			"risks[0].chunk",
		},
		{
			"invalid risk reference",
			Commit{Position: 6, Risks: []Risk{{Reference: "bad\xff"}}},
			"risks[0].reference",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			valid := Commit{Position: 0, Shape: ShapeLayer, Subject: "ok", Risks: []Risk{}}

			var buf bytes.Buffer

			err := EncodeManifest(&buf, []Commit{valid, testCase.commit})

			var utf8Err *InvalidUTF8Error
			if !errors.As(err, &utf8Err) {
				t.Fatalf("EncodeManifest() err = %v, want *InvalidUTF8Error", err)
			}

			if utf8Err.Position != testCase.commit.Position {
				t.Fatalf("Position = %d, want %d", utf8Err.Position, testCase.commit.Position)
			}

			if utf8Err.Field != testCase.wantField {
				t.Fatalf("Field = %q, want %q", utf8Err.Field, testCase.wantField)
			}

			if buf.Len() != 0 {
				t.Fatalf("EncodeManifest() wrote %d bytes before refusing, want 0", buf.Len())
			}
		})
	}
}

func TestEncodeManifestDoesNotEscapeHTMLCharacters(t *testing.T) {
	t.Parallel()

	commit := Commit{
		Position: 0,
		Shape:    ShapeLayer,
		Subject:  `a<b> && c`,
		Risks:    []Risk{},
	}

	var buf bytes.Buffer

	if err := EncodeManifest(&buf, []Commit{commit}); err != nil {
		t.Fatalf("EncodeManifest() err = %v", err)
	}

	line := buf.String()

	for _, want := range []string{"<", ">", "&"} {
		if !strings.Contains(line, want) {
			t.Fatalf(
				"manifest line %s does not contain literal %q: SetEscapeHTML(false) is not in effect",
				line,
				want,
			)
		}
	}

	for _, forbidden := range []string{"\\u003c", "\\u003e", "\\u0026"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf(
				"manifest line %s contains escaped %q, want the literal character",
				line,
				forbidden,
			)
		}
	}
}

func TestEncodeManifestOneLinePerCommit(t *testing.T) {
	t.Parallel()

	commits := []Commit{
		{Position: 0, Shape: ShapeLayer, Subject: "first", Risks: []Risk{}},
		{Position: 1, Shape: ShapeFeature, Subject: "second", Risks: []Risk{}},
	}

	var buf bytes.Buffer

	if err := EncodeManifest(&buf, commits); err != nil {
		t.Fatalf("EncodeManifest() err = %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(commits) {
		t.Fatalf("wrote %d lines, want %d", len(lines), len(commits))
	}
}

func TestManifestMatchesDesignedShape(t *testing.T) {
	t.Parallel()

	commit := Commit{
		Position: 41,
		Shape:    ShapeFeature,
		Feature:  "modules/lead",
		Subject:  "DRAFT: feat(lead): Lead, Create",
		Chunks:   []Chunk{{Key: "modules/lead/lead.go:type:Lead", Span: [2]int{120, 340}}},
		Blobs:    []Blob{{Path: "modules/lead/lead.go", OID: "3f2a...", Mode: ModeFile}},
		Risks:    []Risk{},
	}

	var buf bytes.Buffer

	if err := EncodeManifest(&buf, []Commit{commit}); err != nil {
		t.Fatalf("EncodeManifest() err = %v", err)
	}

	line := strings.TrimSpace(buf.String())

	for _, field := range []string{
		`"position":41`, `"shape":"feature"`, `"feature":"modules/lead"`,
		`"subject":"DRAFT: feat(lead): Lead, Create"`,
		`"key":"modules/lead/lead.go:type:Lead"`, `"span":[120,340]`,
		`"path":"modules/lead/lead.go"`, `"oid":"3f2a..."`, `"mode":"100644"`,
	} {
		if !strings.Contains(line, field) {
			t.Fatalf("manifest line %s does not contain %s", line, field)
		}
	}
}

func TestDecodeManifestPropagatesUnmarshalError(t *testing.T) {
	t.Parallel()

	_, err := DecodeManifest(strings.NewReader("not json\n"))
	if err == nil {
		t.Fatalf("DecodeManifest() err = nil, want an error")
	}
}

func TestDecodeManifestErrorNamesTheFailingLine(t *testing.T) {
	t.Parallel()

	in := `{"position":0,"shape":"layer","subject":"a","risks":[]}` + "\n" +
		"not json\n"

	_, err := DecodeManifest(strings.NewReader(in))

	var lineErr *ManifestLineError
	if !errors.As(err, &lineErr) {
		t.Fatalf("DecodeManifest() err = %v, want *ManifestLineError", err)
	}

	if lineErr.Line != 2 {
		t.Fatalf("Line = %d, want %d", lineErr.Line, 2)
	}
}

func TestDecodeManifestSkipsBlankLines(t *testing.T) {
	t.Parallel()

	in := `{"position":0,"shape":"layer","subject":"a","risks":[]}` + "\n\n" +
		`{"position":1,"shape":"feature","subject":"b","risks":[]}` + "\n"

	got, err := DecodeManifest(strings.NewReader(in))
	if err != nil {
		t.Fatalf("DecodeManifest() err = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("DecodeManifest() returned %d commits, want 2", len(got))
	}
}

type erroringReader struct{ err error }

func (r erroringReader) Read([]byte) (int, error) { return 0, r.err }

func TestDecodeManifestPropagatesReadError(t *testing.T) {
	t.Parallel()

	readErr := errors.New("boom")

	_, err := DecodeManifest(erroringReader{err: readErr})
	if !errors.Is(err, readErr) {
		t.Fatalf("DecodeManifest() err = %v, want %v", err, readErr)
	}
}
