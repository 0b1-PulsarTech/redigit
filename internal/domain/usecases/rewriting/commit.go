package rewriting

// FileContent is the whole content a path holds after a commit, never a diff
// against a prior version: the writer builds each tree from full blobs.
type FileContent struct {
	Path    string
	Content []byte
}

// Commit is one commit to write. Deletes carries the paths that must vanish;
// writing an empty file instead would leave a zero-byte blob in the tree and
// break the final-tree invariant without saying why.
type Commit struct {
	Subject string
	Writes  []FileContent
	Deletes []string
}

// IsEmpty reports a commit that changes nothing. An empty commit wearing a
// feature subject claims work that happened somewhere else.
func (c Commit) IsEmpty() bool { return len(c.Writes) == 0 && len(c.Deletes) == 0 }
