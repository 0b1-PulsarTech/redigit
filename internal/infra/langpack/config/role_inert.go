package config

import (
	"path"
	"slices"
	"strings"
)

// inertDirs hold what a repository writes last or by hand with an owner of
// its own: pipelines and agent instructions are the final commits' business.
var inertDirs = []string{".github", ".gitlab", ".gitea", ".circleci", ".claude"}

// isInert reports a file no scaffold or documentation rule may move: the
// formats other rules own (Go, SQL, go.mod, go.sum, go.work), fixtures a test
// reads, and the pipeline directories.
func isInert(file string) bool {
	switch path.Ext(file) {
	case ".go", ".sql":
		return true
	}

	switch path.Base(file) {
	case goModFile, goSumFile, goWorkFile:
		return true
	}

	segments := strings.Split(file, "/")

	return slices.Contains(segments, "testdata") || slices.Contains(inertDirs, segments[0]) ||
		isAgentInstruction(file)
}

// agentInstructionFiles are the documents a repository keeps for the agents that
// work in it, which describe a finished tree to a tool, not a program to a reader.
var agentInstructionFiles = []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"}

// isAgentInstruction reports a root file written for agents: the conventional
// instruction documents, and the dotfiles whose name says they are rules for one.
func isAgentInstruction(file string) bool {
	if path.Dir(file) != "." {
		return false
	}

	base := path.Base(file)

	return slices.Contains(agentInstructionFiles, base) ||
		strings.HasPrefix(base, ".") && strings.Contains(base, "rules")
}
