package lint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
)

func TestLoadLimits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		want    linting.Limits
		wantErr error
	}{
		{name: "no section keeps the defaults", content: `{}`, want: linting.DefaultLimits()},
		{
			name:    "every limit can be set",
			content: `{"lint":{"sourceAdded":10,"generatedMixed":20,"rowAdded":30}}`,
			want:    linting.Limits{SourceAdded: 10, GeneratedMixed: 20, RowAdded: 30},
		},
		{
			name:    "one limit leaves the others",
			content: `{"lint":{"rowAdded":5}}`,
			want:    linting.Limits{SourceAdded: 900, GeneratedMixed: 900, RowAdded: 5},
		},
		{
			name: "an unknown setting is refused", content: `{"lint":{"rowsAdded":5}}`,
			wantErr: bootstrap.ErrConfig,
		},
		{
			name: "a zero limit is refused", content: `{"lint":{"rowAdded":0}}`,
			wantErr: bootstrap.ErrConfig,
		},
		{
			name: "a text limit is refused", content: `{"lint":{"rowAdded":"5"}}`,
			wantErr: bootstrap.ErrConfig,
		},
		{
			name: "a section that is not an object", content: `{"lint":[1]}`,
			wantErr: bootstrap.ErrConfig,
		},
		{
			name: "a null section is refused", content: `{"lint":null}`,
			wantErr: bootstrap.ErrConfig,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "redigit.json")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatalf("WriteFile() err = %v", err)
			}

			got, err := loadLimits(path)
			if !errors.Is(err, testCase.wantErr) || got != testCase.want {
				t.Fatalf(
					"loadLimits() = %+v, %v, want %+v, %v",
					got,
					err,
					testCase.want,
					testCase.wantErr,
				)
			}
		})
	}
}

func TestLoadLimitsWithoutAFileOrWithAMissingOne(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		path    string
		want    linting.Limits
		wantErr error
	}{
		{name: "no file keeps the defaults", path: "", want: linting.DefaultLimits()},
		{
			name: "a missing file is a config error", path: "/nonexistent/redigit.json",
			wantErr: bootstrap.ErrConfig,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := loadLimits(testCase.path)
			if !errors.Is(err, testCase.wantErr) || got != testCase.want {
				t.Fatalf("loadLimits() = %+v, %v", got, err)
			}
		})
	}
}
