package profiling

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestTimings(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		timings := NewTimings()

		stopGraph := timings.Measure("graph")

		synctest.Sleep(1500 * time.Millisecond)

		stopGraph()

		for range 2 {
			stopPlan := timings.Measure("plan")

			synctest.Sleep(250 * time.Millisecond)

			stopPlan()
		}

		var out bytes.Buffer
		if err := timings.Write(&out); err != nil {
			t.Fatalf("Write() err = %v", err)
		}

		want := "{\"timings\":{\"graph\":1500,\"plan\":500}}\n"
		if out.String() != want {
			t.Fatalf("Write() = %q, want %q", out.String(), want)
		}
	})
}

type failingWriter struct{}

var errWrite = errors.New("write refused")

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }

func TestTimingsWriteFails(t *testing.T) {
	t.Parallel()

	if err := NewTimings().Write(failingWriter{}); err == nil {
		t.Fatal("Write() err = nil, want the writer's failure")
	}
}

func TestTimingsMeasureIsSafeFromManyGoroutines(t *testing.T) {
	t.Parallel()

	const workers = 16

	timings := NewTimings()
	start := make(chan struct{})

	var group sync.WaitGroup

	for idx := range workers {
		group.Go(func() {
			<-start

			stop := timings.Measure("stage-" + strconv.Itoa(idx%4))
			stop()
		})
	}

	close(start)
	group.Wait()

	var out bytes.Buffer
	if err := timings.Write(&out); err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	for idx := range 4 {
		if want := "stage-" + strconv.Itoa(idx); !strings.Contains(out.String(), want) {
			t.Errorf("Write() = %q, want it to hold %s", out.String(), want)
		}
	}
}
