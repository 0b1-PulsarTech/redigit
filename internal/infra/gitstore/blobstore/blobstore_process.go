package blobstore

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// start launches the hash-object process, deliberately outside any single
// Put call's context: the process outlives whichever call happens to trigger
// it and is torn down only by Close. Tying it to a per-call context instead
// kills it the moment that one call's context ends, even mid-batch.
func (s *BlobStore) start() error {
	if s.cmd != nil {
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "redigit-blobstore-")
	if err != nil {
		return fmt.Errorf("create blob store scratch dir: %w", err)
	}

	args := []string{"-C", s.repoPath, "hash-object", "--stdin-paths"}
	if s.write {
		args = append(args, "-w")
	}

	// #nosec G204 -- driving the git CLI is this adapter's purpose; repoPath
	// is caller-supplied infra config, not untrusted input.
	//
	// context.Background(), not a Put call's ctx: this process outlives
	// whichever call happens to start it, and is torn down only by Close.
	cmd := exec.CommandContext(context.Background(), "git", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = os.RemoveAll(tmpDir)

		return fmt.Errorf("open hash-object stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		// cmd.StdinPipe()'s own cleanup only runs via cmd.Start() failing or
		// cmd.Wait() succeeding, neither of which happens here: close it
		// explicitly so this error path does not leak the pipe's fd.
		_ = stdin.Close()
		_ = os.RemoveAll(tmpDir)

		return fmt.Errorf("open hash-object stdout: %w", err)
	}

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	if err = cmd.Start(); err != nil {
		_ = os.RemoveAll(tmpDir)

		return fmt.Errorf("start hash-object: %w", err)
	}

	s.tmpDir = tmpDir
	s.cmd = cmd
	s.stdin = stdin
	s.stdout = bufio.NewScanner(stdout)
	s.stderr = &stderr

	return nil
}

// hash writes content to a scratch file and asks the running hash-object
// process for its id. A scratch file is unavoidable: --stdin-paths reads
// paths, not content, which is the price paid for batching thousands of
// blobs through one process instead of one process per blob.
func (s *BlobStore) hash(content []byte) (string, error) {
	path, err := s.stage(content)
	if err != nil {
		return "", err
	}

	defer func() { _ = os.Remove(path) }()

	if _, err = fmt.Fprintln(s.stdin, path); err != nil {
		return "", fmt.Errorf("request blob id: %w", err)
	}

	if !s.stdout.Scan() {
		if err = s.stdout.Err(); err != nil {
			return "", fmt.Errorf("read blob id: %w", err)
		}

		return "", fmt.Errorf("read blob id: %w", io.ErrUnexpectedEOF)
	}

	return s.stdout.Text(), nil
}

func (s *BlobStore) stage(content []byte) (string, error) {
	s.next++
	path := filepath.Join(s.tmpDir, fmt.Sprintf("%d", s.next))

	if err := os.WriteFile(path, content, scratchFilePerm); err != nil {
		return "", fmt.Errorf("stage blob content: %w", err)
	}

	return path, nil
}
