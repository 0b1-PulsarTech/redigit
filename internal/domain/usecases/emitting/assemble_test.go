package emitting

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestCheckVersionsEndAtTheirChange(t *testing.T) {
	t.Parallel()

	first, second := key("a.go", "A"), key("a.go", "B")
	commits := []entities.PlannedCommit{
		{Position: 0, Subject: "feat: A", Decls: []entities.DeclKey{first}},
		{Position: 1, Subject: "fix: A", Changes: []entities.DeclKey{first}},
		{Position: 2, Subject: "feat: B", Decls: []entities.DeclKey{second}},
	}

	type versions = map[entities.DeclKey][]entities.Version

	testCases := []struct {
		name     string
		versions versions
		wantErr  error
	}{
		{name: "no version"},
		{
			name:     "a version ending at its change",
			versions: versions{first: {{Until: 1}}},
		},
		{
			name:     "a version ending past the end",
			versions: versions{first: {{Until: 3}}}, wantErr: apperr.ErrIncomplete,
		},
		{
			name:     "a version ending at another commit",
			versions: versions{first: {{Until: 2}}}, wantErr: apperr.ErrIncomplete,
		},
		{
			name:     "a version nothing changes",
			versions: versions{second: {{Until: 1}}}, wantErr: apperr.ErrIncomplete,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := check(entities.Plan{Commits: commits, Versions: testCase.versions})
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("check() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestEmitterRefusesAChangeThatIsNotBornYet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		ghost   entities.DeclKey
		wantErr error
	}{
		{
			name:    "a change naming a declaration no commit has born",
			ghost:   key("a.go", "Ghost"),
			wantErr: apperr.ErrIncomplete,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			plan, graph := anchoredFixture()
			plan.Commits[2].Changes = append(plan.Commits[2].Changes, testCase.ghost)

			_, err := New(NewMockSource(ctrl), echoRenderer(ctrl), countingSink(ctrl)).
				Emit(context.Background(), plan, graph)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Emit() error = %v, want ErrIncomplete", err)
			}
		})
	}
}
