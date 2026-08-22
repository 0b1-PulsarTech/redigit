package entities

import "slices"

// SourceTouch is one position of the source history and the paths it wrote.
// It is the only thing the source history contributes: a floor per declaration,
// alongside the module floor and the reader floor. Nothing privileged, because
// the tool also has to work with no source history at all.
type SourceTouch struct {
	Position Position
	// Rev names the commit. It is kept because a declaration's real birth is
	// the first commit whose version of the file already held it, and finding
	// that means reading the file AT that commit - the file's first touch is
	// only where its first few declarations were born.
	Rev     string
	Touched []string
}

// Snapshot is what the run reads: the final tree, and the source history when
// there is one. Files are repo-relative and already filtered to what the run
// covers, so a stage never re-decides what is in scope.
type Snapshot struct {
	Root    string
	Files   []string
	History []SourceTouch
}

// HasHistory reports whether declarations can inherit a base position. Without
// one the order comes from dependency alone, and the feature planner stops being
// a refinement and becomes the only thing keeping the history from going flat.
func (s Snapshot) HasHistory() bool { return len(s.History) > 0 }

// FirstTouch reports the earliest position that wrote a path.
//
// It scans for the minimum rather than returning the first match, so it does
// not depend on History being sorted. Leaving that as an unwritten contract
// makes a caller correct by luck: an unsorted history would silently seed a
// declaration at a later position than the one that really introduced it.
func (s Snapshot) FirstTouch(path string) Position {
	first := NoPosition

	for _, touch := range s.History {
		if !slices.Contains(touch.Touched, path) {
			continue
		}

		if !first.IsSet() || touch.Position < first {
			first = touch.Position
		}
	}

	return first
}

// ChunkSpan is one piece a caller carved out of a file the tool has no reader
// for. Name identifies the piece the way a declaration name does.
type ChunkSpan struct {
	Name string
	Span Span
}

// ChunkMap is a chunking supplied from outside, keyed by repo-relative path.
//
// It exists because guessing how to split an unknown format is worse than not
// splitting it, and because whoever operates this tool knows things it does
// not. The caller says WHERE the cuts go; the tool makes them without missing a
// byte. Anything a map leaves uncovered becomes a piece of its own, so nothing
// disappears because a span forgot it.
type ChunkMap map[string][]ChunkSpan
