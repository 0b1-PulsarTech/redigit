package indexing

import (
	"context"
	"errors"
	"iter"
	"slices"
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

func changes(list ...RevisionChange) iter.Seq2[RevisionChange, error] {
	return func(yield func(RevisionChange, error) bool) {
		for _, change := range list {
			if !yield(change, nil) {
				return
			}
		}
	}
}

func failing(err error) iter.Seq2[RevisionChange, error] {
	return func(yield func(RevisionChange, error) bool) { yield(RevisionChange{}, err) }
}

func goConfig(syntax Syntax) Config {
	return Config{
		Concurrency: 1,
		Languages:   []Language{{Name: "go", Extensions: []string{".go"}, Syntax: syntax}},
	}
}

// The file is touched at position 0 holding one function and again at 40
// holding three. Anchoring on the file dates all three to 0; only reading the
// file as it stood then puts the later two at 40.
func TestScannerDatesEachDeclarationNotTheFile(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	log := NewMockRevisionLog(ctrl)
	log.EXPECT().Changes(gomock.Any()).Return(changes(
		RevisionChange{Rev: "aaa", Files: []ChangedFile{{Path: "lead.go", Blob: "b1"}}},
		RevisionChange{Rev: "bbb", Files: []ChangedFile{{Path: "lead.go", Blob: "b2"}}},
	))
	log.EXPECT().ReadBlob(gomock.Any(), "b1").Return([]byte("early"), nil)
	log.EXPECT().ReadBlob(gomock.Any(), "b2").Return([]byte("late"), nil)

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().
		Declarations(gomock.Any(), "lead.go", []byte("early")).
		Return(grown("lead.go", "First"), nil)
	syntax.EXPECT().
		Declarations(gomock.Any(), "lead.go", []byte("late")).
		Return(grown("lead.go", "First", "Second", "Third"), nil)

	births, err := NewScanner(log, goConfig(syntax)).Scan(context.Background(), entities.Snapshot{
		History: []entities.SourceTouch{{Position: 0, Rev: "aaa"}, {Position: 40, Rev: "bbb"}},
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

func TestScannerParsesASharedBlobOnce(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	log := NewMockRevisionLog(ctrl)
	log.EXPECT().Changes(gomock.Any()).Return(changes(
		RevisionChange{Rev: "aaa", Files: []ChangedFile{{Path: "lead.go", Blob: "same"}}},
		RevisionChange{Rev: "bbb", Files: []ChangedFile{{Path: "lead.go", Blob: "same"}}},
		RevisionChange{Rev: "ccc", Files: []ChangedFile{{Path: "lead.go", Blob: "same"}}},
	))
	log.EXPECT().ReadBlob(gomock.Any(), "same").Return([]byte("text"), nil).Times(1)

	syntax := NewMockSyntax(ctrl)
	syntax.EXPECT().Declarations(gomock.Any(), "lead.go", []byte("text")).
		Return(grown("lead.go", "Only"), nil).Times(1)

	births, err := NewScanner(log, goConfig(syntax)).Scan(context.Background(), entities.Snapshot{
		History: []entities.SourceTouch{
			{Position: 0, Rev: "aaa"}, {Position: 1, Rev: "bbb"}, {Position: 2, Rev: "ccc"},
		},
	})
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	key := entities.DeclKey{File: "lead.go", Kind: entities.KindFunc, Name: "Only"}
	if got := births.At[key]; got != 0 {
		t.Fatalf("Only born at %d, want 0", got)
	}
}

func TestScannerReportsWhatItCouldNotRead(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		files       []ChangedFile
		readErr     error
		parseErr    error
		wantRead    bool
		wantParse   bool
		wantPending []string
	}{
		{
			name:        "deleted path",
			files:       []ChangedFile{{Path: "gone.go", IsDeleted: true}},
			wantPending: []string{"aaa:gone.go"},
		},
		{
			name:        "blob unreadable",
			files:       []ChangedFile{{Path: "a.go", Blob: "b1"}},
			readErr:     ErrBlobMissing,
			wantRead:    true,
			wantPending: []string{"aaa:a.go"},
		},
		{
			name:        "does not parse as it stood",
			files:       []ChangedFile{{Path: "a.go", Blob: "b1"}},
			parseErr:    errors.New("syntax"),
			wantRead:    true,
			wantParse:   true,
			wantPending: []string{"aaa:a.go"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			log := NewMockRevisionLog(ctrl)
			syntax := NewMockSyntax(ctrl)

			log.EXPECT().Changes(gomock.Any()).
				Return(changes(RevisionChange{Rev: "aaa", Files: testCase.files}))

			if testCase.wantRead {
				log.EXPECT().ReadBlob(gomock.Any(), "b1").Return([]byte("x"), testCase.readErr)
			}

			if testCase.wantParse {
				syntax.EXPECT().Declarations(gomock.Any(), "a.go", []byte("x")).
					Return(nil, testCase.parseErr)
			}

			births, err := NewScanner(log, goConfig(syntax)).Scan(
				context.Background(),
				entities.Snapshot{History: []entities.SourceTouch{{Position: 0, Rev: "aaa"}}},
			)
			if err != nil {
				t.Fatalf("Scan() = %v", err)
			}

			if len(births.Unreadable) != 1 || births.Unreadable[0] != testCase.wantPending[0] {
				t.Fatalf("Unreadable = %v, want %v", births.Unreadable, testCase.wantPending)
			}

			if len(births.At) != 0 {
				t.Fatalf("At = %v, want nothing dated", births.At)
			}
		})
	}
}

func TestScannerSkipsUnownedFiles(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	log := NewMockRevisionLog(ctrl)
	log.EXPECT().Changes(gomock.Any()).Return(changes(RevisionChange{
		Rev: "aaa", Files: []ChangedFile{{Path: "README.md"}, {Path: "query.sql"}},
	}))

	births, err := NewScanner(log, goConfig(NewMockSyntax(ctrl))).Scan(
		context.Background(),
		entities.Snapshot{History: []entities.SourceTouch{{Position: 0, Rev: "aaa"}}},
	)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	if len(births.At) != 0 || len(births.Unreadable) != 0 {
		t.Fatalf("scan touched files it does not own: %+v", births)
	}
}

func TestScannerFailures(t *testing.T) {
	t.Parallel()

	logErr := errors.New("log died")
	errReadBlob := errors.New("process died")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	testCases := []struct {
		name    string
		snap    entities.Snapshot
		ctx     context.Context
		stream  iter.Seq2[RevisionChange, error]
		wantErr error
	}{
		{
			name:    "no history",
			snap:    entities.Snapshot{Files: []string{"a.go"}},
			ctx:     context.Background(),
			wantErr: errNoHistory,
		},
		{
			name:    "log stream fails",
			snap:    entities.Snapshot{History: []entities.SourceTouch{{Rev: "aaa"}}},
			ctx:     context.Background(),
			stream:  failing(logErr),
			wantErr: logErr,
		},
		{
			name: "blob read fails for a live reason",
			snap: entities.Snapshot{History: []entities.SourceTouch{{Rev: "aaa"}}},
			ctx:  context.Background(),
			stream: changes(RevisionChange{
				Rev: "aaa", Files: []ChangedFile{{Path: "a.go", Blob: "b1"}},
			}),
			wantErr: errReadBlob,
		},
		{
			name: "canceled while parsing",
			snap: entities.Snapshot{History: []entities.SourceTouch{{Rev: "aaa"}}},
			ctx:  canceled,
			stream: changes(RevisionChange{
				Rev: "aaa", Files: []ChangedFile{{Path: "a.go", Blob: "b1"}},
			}),
			wantErr: context.Canceled,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			log := NewMockRevisionLog(ctrl)

			if testCase.stream != nil {
				log.EXPECT().Changes(gomock.Any()).Return(testCase.stream)
			}

			log.EXPECT().ReadBlob(gomock.Any(), gomock.Any()).
				Return(nil, errReadBlob).AnyTimes()

			scanner := NewScanner(log, goConfig(NewMockSyntax(ctrl)))

			_, err := scanner.Scan(testCase.ctx, testCase.snap)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Scan() = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestScannerSortsWhatItCouldNotRead(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	log := NewMockRevisionLog(ctrl)

	log.EXPECT().Changes(gomock.Any()).Return(changes(RevisionChange{
		Rev: "aaa", Files: []ChangedFile{
			{Path: "z.go", IsDeleted: true}, {Path: "a.go", IsDeleted: true},
		},
	}))

	births, err := NewScanner(log, goConfig(NewMockSyntax(ctrl))).Scan(
		context.Background(),
		entities.Snapshot{History: []entities.SourceTouch{{Position: 0, Rev: "aaa"}}},
	)
	if err != nil {
		t.Fatalf("Scan() = %v", err)
	}

	if want := []string{"aaa:a.go", "aaa:z.go"}; !slices.Equal(births.Unreadable, want) {
		t.Fatalf("Unreadable = %v, want %v", births.Unreadable, want)
	}
}
