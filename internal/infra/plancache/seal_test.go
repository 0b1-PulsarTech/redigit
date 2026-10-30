package plancache

import (
	"crypto/sha256"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestWriteSealedSyncsThenClosesOnce(t *testing.T) {
	t.Parallel()

	payload := []byte("entry")
	errWrite := errors.New("write")
	errSync := errors.New("sync")
	errClose := errors.New("close")

	testCases := []struct {
		name     string
		writeErr error
		syncErr  error
		closeErr error
		synced   bool
		wantErrs []error
	}{
		{"all of it works", nil, nil, nil, true, nil},
		{"the write fails", errWrite, nil, errClose, false, []error{errWrite, errClose}},
		{"the sync fails", nil, errSync, errClose, true, []error{errSync, errClose}},
		{"only the close fails", nil, nil, errClose, true, []error{errClose}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			out := NewMocksealedFile(gomock.NewController(t))
			sealedLen := len(payload) + sha256.Size

			written := out.EXPECT().
				Write(gomock.Len(sealedLen)).
				Return(sealedLen, testCase.writeErr)
			previous := written

			if testCase.synced {
				previous = out.EXPECT().Sync().Return(testCase.syncErr).After(written)
			}

			out.EXPECT().Close().Return(testCase.closeErr).Times(1).After(previous)

			err := writeSealed(out, payload)

			if len(testCase.wantErrs) == 0 && err != nil {
				t.Fatalf("writeSealed() err = %v, want nil", err)
			}

			for _, want := range testCase.wantErrs {
				if !errors.Is(err, want) {
					t.Errorf("writeSealed() err = %v, want it to hold %v", err, want)
				}
			}
		})
	}
}
