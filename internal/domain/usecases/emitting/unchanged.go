package emitting

import (
	"log/slog"
	"strings"
)

type held map[string]Handle

func (latest held) observe(commit Commit) {
	if latest.repeats(commit.Blobs) {
		keys := make([]string, 0, len(commit.Chunks))
		for _, chunk := range commit.Chunks {
			keys = append(keys, chunk.Key.String())
		}

		// A warning and not a refusal: other causes may exist, and it keeps them visible.
		slog.Warn(
			"commit leaves every file as the last row held it",
			slog.Int("position", int(commit.Position)),
			slog.String("chunks", strings.Join(keys, ", ")),
		)
	}

	for _, blob := range commit.Blobs {
		latest[blob.Path] = blob.Handle
	}
}

func (latest held) repeats(blobs []Blob) bool {
	for _, blob := range blobs {
		if previous, seen := latest[blob.Path]; !seen || previous != blob.Handle {
			return false
		}
	}

	return true
}
