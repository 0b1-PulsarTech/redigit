package rewriting

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"
)

func replayRows() []Row {
	return []Row{
		{
			Position: 1,
			Subject:  "second",
			Blobs:    []BlobRef{{Path: "b", Mode: "100644", Handle: "h2"}},
		},
		{Position: 0, Subject: "first"},
	}
}

func TestReplayerReplayOrdersAndVerifies(t *testing.T) {
	t.Parallel()

	staged := Staged{Tip: "tip", Base: "base", Scratch: "refs/redigit/write/1-a"}

	testCases := []struct {
		name       string
		verify     string
		wantVerify bool
	}{
		{"verifies the staged tip against the rev", "want", true},
		{"no verify rev skips the check", "", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			dates := NewMockDateSource(ctrl)
			history := NewMockReplayHistory(ctrl)

			var replayed []Replayed

			dates.EXPECT().AuthorDates(gomock.Any(), "dates").
				Return([]Stamp{{100, "+0100"}, {200, "-0300"}}, nil)
			history.EXPECT().Replay(gomock.Any(), "refs/heads/out", gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, commits []Replayed) (Staged, error) {
					replayed = commits

					return staged, nil
				})

			if testCase.wantVerify {
				history.EXPECT().Verify(gomock.Any(), "tip", "want").Return(nil)
			}

			history.EXPECT().Publish(gomock.Any(), "refs/heads/out", staged).Return(nil)
			history.EXPECT().Discard(gomock.Any(), staged).Return(nil)

			got, err := NewReplayer(dates, history).Replay(t.Context(), ReplayRequest{
				Ref:     "refs/heads/out",
				DateRef: "dates",
				Verify:  testCase.verify,
				Rows:    replayRows(),
			})
			if err != nil {
				t.Fatalf("Replay() err = %v", err)
			}

			if got.Commit != "tip" || got.Commits != 2 {
				t.Fatalf("Replay() = %+v, want tip over 2 commits", got)
			}

			positions := []int{replayed[0].Position, replayed[1].Position}
			if !slices.Equal(positions, []int{0, 1}) || replayed[1].At != (Stamp{200, "-0300"}) ||
				replayed[0].Message != "first\n" || len(replayed[1].Blobs) != 1 {
				t.Fatalf("Replay() sent %+v", replayed)
			}
		})
	}
}

func TestReplayerReplayFailures(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	stamps := []Stamp{{100, "+0100"}}

	testCases := []struct {
		name    string
		rows    []Row
		setup   func(dates *MockDateSource, history *MockReplayHistory)
		wantErr error
	}{
		{
			"empty manifest stops before any port", nil,
			func(*MockDateSource, *MockReplayHistory) {}, ErrManifest,
		},
		{
			"duplicate position stops before any port",
			[]Row{{Position: 1}, {Position: 1}},
			func(*MockDateSource, *MockReplayHistory) {}, ErrManifest,
		},
		{
			"dates error stops before replay", replayRows(),
			func(dates *MockDateSource, _ *MockReplayHistory) {
				dates.EXPECT().AuthorDates(gomock.Any(), gomock.Any()).Return(nil, errBoom)
			}, errBoom,
		},
		{
			"empty date source stops before replay", replayRows(),
			func(dates *MockDateSource, _ *MockReplayHistory) {
				dates.EXPECT().AuthorDates(gomock.Any(), gomock.Any()).Return(nil, nil)
			}, ErrNoDates,
		},
		{
			"history error surfaces and publishes nothing", replayRows(),
			func(dates *MockDateSource, history *MockReplayHistory) {
				dates.EXPECT().AuthorDates(gomock.Any(), gomock.Any()).Return(stamps, nil)
				history.EXPECT().
					Replay(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(Staged{}, errBoom)
			}, errBoom,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			dates := NewMockDateSource(ctrl)
			history := NewMockReplayHistory(ctrl)
			testCase.setup(dates, history)

			got, err := NewReplayer(dates, history).Replay(t.Context(), ReplayRequest{
				Ref: "refs/heads/out", DateRef: "dates", Verify: "want", Rows: testCase.rows,
			})
			if !errors.Is(err, testCase.wantErr) || got.Commit != "" {
				t.Fatalf("Replay() = %+v, %v, want %v and no commit", got, err, testCase.wantErr)
			}
		})
	}
}

func TestReplayerReplayFailuresAfterStaging(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	errCleanup := errors.New("cleanup")
	staged := Staged{Tip: "tip", Scratch: "scratch"}

	testCases := []struct {
		name       string
		verifyErr  error
		publishErr error
		discardErr error
		isVerified bool
		wantErrs   []error
		wantCommit string
		wantWarned error
	}{
		{"verify failure never publishes", errBoom, nil, nil, false, []error{errBoom}, "", nil},
		{"publish refusal surfaces", nil, errBoom, nil, true, []error{errBoom}, "", nil},
		{
			"discard failure joins the verify failure",
			errBoom, nil, errCleanup, false,
			[]error{errBoom, errCleanup},
			"", nil,
		},
		{
			"discard failure after a publish is a warning",
			nil, nil, errCleanup, true, nil, "tip", errCleanup,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			dates := NewMockDateSource(ctrl)
			history := NewMockReplayHistory(ctrl)

			stamps := []Stamp{{100, "+0100"}}

			dates.EXPECT().AuthorDates(gomock.Any(), gomock.Any()).Return(stamps, nil)
			history.EXPECT().Replay(gomock.Any(), gomock.Any(), gomock.Any()).Return(staged, nil)
			history.EXPECT().Verify(gomock.Any(), "tip", "want").Return(testCase.verifyErr)
			history.EXPECT().Discard(gomock.Any(), staged).Return(testCase.discardErr)

			if testCase.isVerified {
				history.EXPECT().
					Publish(gomock.Any(), gomock.Any(), staged).
					Return(testCase.publishErr)
			}

			got, err := NewReplayer(dates, history).Replay(t.Context(), ReplayRequest{
				Ref: "refs/heads/out", DateRef: "dates", Verify: "want", Rows: replayRows(),
			})
			for _, want := range testCase.wantErrs {
				if !errors.Is(err, want) {
					t.Fatalf("Replay() err = %v, want %v", err, want)
				}
			}

			if len(testCase.wantErrs) == 0 && err != nil {
				t.Fatalf("Replay() err = %v, want none", err)
			}

			if got.Commit != testCase.wantCommit ||
				!errors.Is(got.CleanupErr, testCase.wantWarned) {
				t.Fatalf(
					"Replay() = %+v, want commit %q and warning %v",
					got, testCase.wantCommit, testCase.wantWarned,
				)
			}
		})
	}
}

func TestReplayerDiscardsWithoutCancel(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	dates := NewMockDateSource(ctrl)
	history := NewMockReplayHistory(ctrl)
	staged := Staged{Tip: "tip", Scratch: "scratch"}
	ctx, cancel := context.WithCancel(t.Context())

	dates.EXPECT().AuthorDates(gomock.Any(), gomock.Any()).Return([]Stamp{{100, "+0100"}}, nil)
	history.EXPECT().Replay(gomock.Any(), gomock.Any(), gomock.Any()).Return(staged, nil)
	history.EXPECT().Verify(gomock.Any(), "tip", "want").DoAndReturn(
		func(context.Context, string, string) error {
			cancel()

			return context.Canceled
		},
	)

	var discardErr error

	history.EXPECT().Discard(gomock.Any(), staged).DoAndReturn(
		func(discardCtx context.Context, _ Staged) error {
			discardErr = discardCtx.Err()

			return nil
		},
	)

	_, err := NewReplayer(dates, history).Replay(ctx, ReplayRequest{
		Ref: "refs/heads/out", DateRef: "dates", Verify: "want", Rows: replayRows(),
	})
	if !errors.Is(err, context.Canceled) || discardErr != nil {
		t.Fatalf(
			"Replay() err = %v, discard ctx err = %v, want the cancel and a live ctx",
			err,
			discardErr,
		)
	}
}

func TestReplayerPublishIsShieldedFromCancel(t *testing.T) {
	t.Parallel()

	staged := Staged{Tip: "tip", Scratch: "scratch"}

	testCases := []struct {
		name            string
		isCanceledEarly bool
		wantErr         error
		wantCommit      string
	}{
		{"cancel before the move refuses it", true, context.Canceled, ""},
		{"cancel during the move does not reach it", false, nil, "tip"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			dates := NewMockDateSource(ctrl)
			history := NewMockReplayHistory(ctrl)
			ctx, cancel := context.WithCancel(t.Context())

			var publishCtxErr error

			dates.EXPECT().
				AuthorDates(gomock.Any(), gomock.Any()).
				Return([]Stamp{{100, "+0100"}}, nil)
			history.EXPECT().Replay(gomock.Any(), gomock.Any(), gomock.Any()).Return(staged, nil)
			history.EXPECT().Verify(gomock.Any(), "tip", "want").DoAndReturn(
				func(context.Context, string, string) error {
					if testCase.isCanceledEarly {
						cancel()
					}

					return nil
				},
			)
			history.EXPECT().Discard(gomock.Any(), staged).Return(nil)

			if !testCase.isCanceledEarly {
				history.EXPECT().Publish(gomock.Any(), gomock.Any(), staged).DoAndReturn(
					func(publishCtx context.Context, _ string, _ Staged) error {
						cancel()

						publishCtxErr = publishCtx.Err()

						return nil
					},
				)
			}

			got, err := NewReplayer(dates, history).Replay(ctx, ReplayRequest{
				Ref: "refs/heads/out", DateRef: "dates", Verify: "want", Rows: replayRows(),
			})
			if !errors.Is(err, testCase.wantErr) || publishCtxErr != nil ||
				got.Commit != testCase.wantCommit {
				t.Fatalf(
					"Replay() = %+v, %v with publish ctx err %v, want commit %q and %v",
					got, err, publishCtxErr, testCase.wantCommit, testCase.wantErr,
				)
			}
		})
	}
}
