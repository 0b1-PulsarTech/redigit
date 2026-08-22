package indexing

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func grown(file string, names ...string) []entities.Decl {
	out := make([]entities.Decl, 0, len(names))
	for _, name := range names {
		out = append(out, entities.Decl{
			Key: entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name},
		})
	}

	return out
}

// TestScannerDatesEachDeclarationNotTheFile is the whole reason this exists.
// The file is touched at position 0 holding one function and again at 40
// holding three. Anchoring on the file dates all three to 0; only reading the
// file as it stood then puts the later two at 40.
func TestScannerDatesEachDeclarationNotTheFile(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	tree := NewMockRevisionTree(ctrl)
	tree.EXPECT().ReadAt(gomock.Any(), "aaa", "lead.go").Return([]byte("early"), nil)
	tree.EXPECT().ReadAt(gomock.Any(), "bbb", "lead.go").Return([]byte("late"), nil)

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().
		Declarations(gomock.Any(), "lead.go", []byte("early")).
		Return(grown("lead.go", "First"), nil)
	syntax.EXPECT().
		Declarations(gomock.Any(), "lead.go", []byte("late")).
		Return(grown("lead.go", "First", "Second", "Third"), nil)

	scanner := NewScanner(tree, Config{
		Concurrency: 1,
		Languages:   []Language{{Name: "go", Extensions: []string{".go"}, Syntax: syntax}},
	})

	births, err := scanner.Scan(context.Background(), entities.Snapshot{
		History: []entities.SourceTouch{
			{Position: 0, Rev: "aaa", Touched: []string{"lead.go"}},
			{Position: 40, Rev: "bbb", Touched: []string{"lead.go"}},
		},
	})
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	testCases := []struct {
		name string
		decl string
		want entities.Position
	}{
		{"the declaration the file was born with", "First", 0},
		{"one the file gained later is not dated to the file's birth", "Second", 40},
		{"nor is the other", "Third", 40},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			key := entities.DeclKey{File: "lead.go", Kind: entities.KindFunc, Name: testCase.decl}
			if got := births.At[key]; got != testCase.want {
				t.Fatalf("%s born at %d, want %d", testCase.decl, got, testCase.want)
			}
		})
	}
}

func TestScannerReportsWhatItCouldNotRead(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	tree := NewMockRevisionTree(ctrl)
	tree.EXPECT().ReadAt(gomock.Any(), "aaa", "gone.go").Return(nil, errors.New("absent"))

	syntax := NewMockSyntax(ctrl)

	births, err := NewScanner(tree, Config{
		Concurrency: 1,
		Languages:   []Language{{Name: "go", Extensions: []string{".go"}, Syntax: syntax}},
	}).Scan(context.Background(), entities.Snapshot{
		History: []entities.SourceTouch{{Position: 0, Rev: "aaa", Touched: []string{"gone.go"}}},
	})
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	// A skipped file is named. A scan that silently dropped a tenth of the
	// history would still hand back a plausible-looking map.
	if len(births.Unreadable) != 1 || births.Unreadable[0] != "aaa:gone.go" {
		t.Fatalf("Unreadable = %v, want the skipped pair named", births.Unreadable)
	}

	if len(births.At) != 0 {
		t.Fatalf("At = %v, want nothing dated", births.At)
	}
}

func TestScannerSkipsUnownedFiles(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	births, err := NewScanner(NewMockRevisionTree(ctrl), Config{
		Languages: []Language{{
			Name:       "go",
			Extensions: []string{".go"},
			Syntax:     NewMockSyntax(ctrl),
		}},
	}).Scan(context.Background(), entities.Snapshot{
		History: []entities.SourceTouch{
			{Position: 0, Rev: "aaa", Touched: []string{"README.md", "query.sql"}},
		},
	})
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	if len(births.At) != 0 || len(births.Unreadable) != 0 {
		t.Fatalf("scan touched files it does not own: %+v", births)
	}
}

func TestScannerNeedsAHistory(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	_, err := NewScanner(NewMockRevisionTree(ctrl), Config{}).
		Scan(context.Background(), entities.Snapshot{Files: []string{"a.go"}})
	if !errors.Is(err, errNoHistory) {
		t.Fatalf("Scan() = %v, want %v", err, errNoHistory)
	}
}
