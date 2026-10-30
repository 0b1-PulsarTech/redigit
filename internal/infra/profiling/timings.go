package profiling

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"sync"
	"time"
)

type Timings struct {
	mu     sync.Mutex
	stages map[string]float64
}

func NewTimings() *Timings {
	return &Timings{stages: map[string]float64{}}
}

func (timings *Timings) Measure(stage string) func() {
	began := time.Now()

	return func() {
		elapsed := float64(time.Since(began)) / float64(time.Millisecond)

		timings.mu.Lock()
		defer timings.mu.Unlock()

		timings.stages[stage] += elapsed
	}
}

func (timings *Timings) Write(out io.Writer) error {
	timings.mu.Lock()
	snapshot := maps.Clone(timings.stages)
	timings.mu.Unlock()

	line := struct {
		Timings map[string]float64 `json:"timings"`
	}{snapshot}

	if err := json.NewEncoder(out).Encode(line); err != nil {
		return fmt.Errorf("write timings: %w", err)
	}

	return nil
}
