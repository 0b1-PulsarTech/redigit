package format

import (
	"context"
	"fmt"
	"testing"
)

func TestRounds(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		units []unit
		want  string
	}{
		{name: "no unit", units: nil, want: "[]"},
		{
			name:  "a path repeated goes to a later round",
			units: []unit{{"h1", "a.go"}, {"h2", "b.go"}, {"h3", "a.go"}},
			want:  "[[0 1] [2]]",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := fmt.Sprint(rounds(testCase.units)); got != testCase.want {
				t.Fatalf("rounds() = %s, want %s", got, testCase.want)
			}
		})
	}
}

func TestReadBlobRefusesAHandleThatIsNotAFileName(t *testing.T) {
	t.Parallel()

	fix := stage(t, []string{`{"position":1}`}, map[string]string{"h1": "x"}, fakeConfig)

	testCases := []struct {
		name    string
		handle  string
		want    string
		wantErr bool
	}{
		{name: "a handle", handle: "h1", want: "x", wantErr: false},
		{name: "a missing handle", handle: "h9", want: "", wantErr: true},
		{name: "a parent", handle: "../m.jsonl", want: "", wantErr: true},
		{name: "a nested name", handle: "a/b", want: "", wantErr: true},
		{name: "an absolute name", handle: "/etc/hostname", want: "", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := readBlob(fix.blobs, testCase.handle)
			if (err != nil) != testCase.wantErr || string(got) != testCase.want {
				t.Fatalf("readBlob() = %q, %v", got, err)
			}
		})
	}
}

func TestFormatUnitsRefusesAMissingClaimedBlob(t *testing.T) {
	t.Parallel()

	fix := stage(t, []string{`{"position":1}`}, map[string]string{"h1": "x"}, fakeConfig)

	set, err := loadTools(fix.config)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name    string
		units   []unit
		wantErr bool
	}{
		{name: "a present blob", units: []unit{{handle: "h1", path: "a.go"}}, wantErr: false},
		{name: "a missing blob", units: []unit{{handle: "h9", path: "a.go"}}, wantErr: true},
		{
			name:    "a missing blob after a present one",
			units:   []unit{{handle: "h1", path: "a.go"}, {handle: "h9", path: "b.go"}},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			verdicts, formatErr := formatUnits(
				context.Background(), set.engine, fix.blobs, testCase.units,
			)
			if (formatErr != nil) != testCase.wantErr || (formatErr != nil && verdicts != nil) {
				t.Fatalf("formatUnits() = %v, %v", verdicts, formatErr)
			}
		})
	}
}
