package verify

import (
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

type (
	rowLine struct {
		Position int    `json:"position"`
		Commit   string `json:"commit"`
		Subject  string `json:"subject"`
		Module   string `json:"module"`
		Language string `json:"language"`
		Status   string `json:"status"`
		Step     string `json:"step"`
		IsWip    bool   `json:"wip"`
		IsCached bool   `json:"cached"`
		Millis   int64  `json:"millis"`
		Tail     string `json:"tail,omitempty"`
	}

	summaryLine struct {
		Summary        bool  `json:"summary"`
		Commits        int   `json:"commits"`
		Checked        int   `json:"checked"`
		Passed         int   `json:"passed"`
		Failed         int   `json:"failed"`
		WipFailed      int   `json:"wip_failed"`
		Empty          int   `json:"empty"`
		Cached         int   `json:"cached"`
		OverheadMicros int64 `json:"overhead_micros"`
		ElapsedMillis  int64 `json:"elapsed_ms"`
		IsOK           bool  `json:"ok"`
	}
)

func newRowLine(row verifying.Row) rowLine {
	return rowLine{
		Position: row.Position, Commit: row.Commit, Subject: row.Subject, Module: row.Module,
		Language: row.Language, Status: string(row.Status), Step: row.Step, IsWip: row.IsWip,
		IsCached: row.IsCached, Millis: row.Millis, Tail: row.Tail,
	}
}

func newSummaryLine(summary verifying.Summary, elapsed time.Duration) summaryLine {
	return summaryLine{
		Summary: true, Commits: summary.Commits, Checked: summary.Checked,
		Passed: summary.Passed, Failed: summary.Failed, WipFailed: summary.WipFailed,
		Empty: summary.Empty, Cached: summary.Cached, OverheadMicros: summary.OverheadMicros,
		ElapsedMillis: elapsed.Milliseconds(), IsOK: summary.Failed == 0 && summary.Checked > 0,
	}
}
