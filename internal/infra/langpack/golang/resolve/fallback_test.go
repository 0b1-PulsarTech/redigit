package resolve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var errSecondary = errors.New("secondary failed")

func quietLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestFallbackAsks(t *testing.T) {
	t.Parallel()

	at := entities.Location{File: "a.go", Line: 3}
	answer := []entities.Location{{File: "b.go", Line: 1}}
	fromSecondary := []entities.Location{{File: "c.go", Line: 2}}
	miss := fmt.Errorf("wrapped: %w", ErrNotCovered)

	testCases := []struct {
		name          string
		primaryErr    error
		secondaryErr  error
		wantSecondary bool
		wantCount     int64
		wantErr       error
	}{
		{"the primary answers", nil, nil, false, 0, nil},
		{"the primary fails for another reason", errSecondary, nil, false, 0, errSecondary},
		{"not covered falls through", miss, nil, true, 1, nil},
		{"not covered and the secondary fails", miss, errSecondary, true, 1, errSecondary},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			primary, secondary := NewMockSecondary(ctrl), NewMockSecondary(ctrl)

			primary.EXPECT().References(gomock.Any(), at).Return(answer, testCase.primaryErr)
			primary.EXPECT().Implementations(gomock.Any(), at).Return(answer, testCase.primaryErr)
			primary.EXPECT().Definition(gomock.Any(), at).Return(answer[0], testCase.primaryErr)

			if testCase.wantSecondary {
				secondary.EXPECT().
					References(gomock.Any(), at).
					Return(fromSecondary, testCase.secondaryErr)
				secondary.EXPECT().
					Implementations(gomock.Any(), at).
					Return(fromSecondary, testCase.secondaryErr)
				secondary.EXPECT().
					Definition(gomock.Any(), at).
					Return(fromSecondary[0], testCase.secondaryErr)
			}

			fallback := NewFallback(primary, secondary, quietLogger())
			ctx := context.Background()

			refs, refsErr := fallback.References(ctx, at)
			impls, implsErr := fallback.Implementations(ctx, at)
			def, defErr := fallback.Definition(ctx, at)

			for _, err := range []error{refsErr, implsErr, defErr} {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("error = %v, want %v", err, testCase.wantErr)
				}
			}

			want := answer
			if testCase.wantSecondary {
				want = fromSecondary
			}

			if !slices.Equal(refs, want) || !slices.Equal(impls, want) || def != want[0] {
				t.Fatalf("answers = %v, %v, %v; want %v", refs, impls, def, want)
			}

			if got := fallback.Fallbacks(); got != 3*testCase.wantCount {
				t.Fatalf("Fallbacks() = %d, want %d", got, 3*testCase.wantCount)
			}
		})
	}
}

func TestFallbackClose(t *testing.T) {
	t.Parallel()

	errPrimary := errors.New("primary would not stop")

	testCases := []struct {
		name         string
		primaryErr   error
		secondaryErr error
	}{
		{"both close", nil, nil},
		{"the secondary fails to close", nil, errSecondary},
		{"the primary fails to close", errPrimary, nil},
		{"both fail, and neither hides the other", errPrimary, errSecondary},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			primary := NewMockSecondary(gomock.NewController(t))
			primary.EXPECT().Close(gomock.Any()).Return(testCase.primaryErr)

			secondary := NewMockSecondary(gomock.NewController(t))
			secondary.EXPECT().Close(gomock.Any()).Return(testCase.secondaryErr)

			err := NewFallback(primary, secondary, quietLogger()).Close(context.Background())

			for _, target := range []error{testCase.primaryErr, testCase.secondaryErr} {
				if target != nil && !errors.Is(err, target) {
					t.Fatalf("Close() error = %v, want it to wrap %v", err, target)
				}
			}

			if testCase.primaryErr == nil && testCase.secondaryErr == nil && err != nil {
				t.Fatalf("Close() error = %v, want nil", err)
			}
		})
	}
}

func TestFallbackCountsUnderContention(t *testing.T) {
	t.Parallel()

	const callers = 16

	ctrl := gomock.NewController(t)
	primary, secondary := NewMockSecondary(ctrl), NewMockSecondary(ctrl)
	primary.EXPECT().Definition(gomock.Any(), gomock.Any()).
		Return(entities.Location{}, ErrNotCovered).Times(callers)
	secondary.EXPECT().
		Definition(gomock.Any(), gomock.Any()).
		Return(entities.Location{}, nil).
		Times(callers)

	fallback := NewFallback(primary, secondary, quietLogger())
	begin := make(chan struct{})

	var group sync.WaitGroup

	for range callers {
		group.Go(func() {
			<-begin

			_, _ = fallback.Definition(context.Background(), entities.Location{})
		})
	}

	close(begin)
	group.Wait()

	if got := fallback.Fallbacks(); got != callers {
		t.Fatalf("Fallbacks() = %d, want %d", got, callers)
	}
}

func TestFallbackKeepsAFailedStart(t *testing.T) {
	t.Parallel()

	failedStart := fmt.Errorf("%w: %w", ErrSecondaryStart, errSecondary)

	testCases := []struct {
		name         string
		primaryErr   error
		secondaryErr error
		wantErr      error
	}{
		{"a secondary that does not start", ErrNotCovered, failedStart, ErrSecondaryStart},
		{"another secondary failure is not a start failure", ErrNotCovered, errSecondary, nil},
		{"a primary that answers never starts the secondary", nil, nil, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			primary, secondary := NewMockSecondary(ctrl), NewMockSecondary(ctrl)

			queries := 1
			if testCase.primaryErr != nil {
				queries = 2
				secondary.EXPECT().Definition(gomock.Any(), gomock.Any()).
					Return(entities.Location{}, testCase.secondaryErr).Times(queries)
			}

			primary.EXPECT().Definition(gomock.Any(), gomock.Any()).
				Return(entities.Location{}, testCase.primaryErr).Times(queries)

			fallback := NewFallback(primary, secondary, quietLogger())

			for range queries {
				_, _ = fallback.Definition(context.Background(), entities.Location{})
			}

			if err := fallback.Err(); !errors.Is(err, testCase.wantErr) ||
				(testCase.wantErr == nil && err != nil) {
				t.Fatalf("Err() = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

type adhocPrimary struct {
	*MockSecondary

	adhocErr error
	settled  *bool
}

func (primary adhocPrimary) Close(ctx context.Context) error {
	*primary.settled = true

	return primary.MockSecondary.Close(ctx)
}

func (primary adhocPrimary) AdhocErr() error {
	if !*primary.settled {
		return nil
	}

	return primary.adhocErr
}

func TestFallbackCloseLogsAnAdhocFailure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		adhocErr error
		want     int
	}{
		{"an ad hoc pass that failed", errSecondary, 1},
		{"an ad hoc pass that did not", nil, 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			secondary := NewMockSecondary(ctrl)
			secondary.EXPECT().Close(gomock.Any()).Return(nil)

			var logged bytes.Buffer

			primary := adhocPrimary{
				MockSecondary: NewMockSecondary(ctrl),
				adhocErr:      testCase.adhocErr,
				settled:       new(bool),
			}
			primary.EXPECT().Close(gomock.Any()).Return(nil)
			fallback := NewFallback(primary, secondary, slog.New(slog.NewTextHandler(&logged, nil)))

			if err := fallback.Close(context.Background()); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			const warning = `level=WARN msg="typed resolver skipped ad hoc files"`

			if got := strings.Count(logged.String(), warning); got != testCase.want {
				t.Fatalf(
					"logged %d ad hoc warnings, want %d: %s",
					got,
					testCase.want,
					logged.String(),
				)
			}
		})
	}
}
