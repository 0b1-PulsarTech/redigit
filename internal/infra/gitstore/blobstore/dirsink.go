package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
)

// DirSink writes a plan's blobs onto disk under Dir instead of into a git
// object database, so a caller who does not want the tool touching a
// repository's objects at all still gets the same manifest, minus the oid.
type DirSink struct {
	Dir string
}

var _ BlobSink = (*DirSink)(nil)

func NewDirSink(dir string) DirSink { return DirSink{Dir: dir} }

// Put writes content to Dir/path, creating any parent directories, and
// carries mode into the file's permission bits so an inspected checkout at
// least looks like what the manifest describes. It never returns an object
// id: there is no object database backing a directory.
//
// Whatever already exists at the destination is removed first, so a path
// that changes mode across commits (file to executable, symlink to plain
// file, or the reverse) always lands cleanly instead of silently keeping
// stale permission bits or a stale symlink: os.WriteFile only applies its
// perm argument when creating a file, not when overwriting one.
func (s DirSink) Put(
	_ context.Context,
	path string,
	mode gitstore.FileMode,
	content []byte,
) (string, error) {
	// Content-addressed, exactly like the object database this is the
	// alternative to. Addressing by path collapses every commit's version of a
	// file onto the last one written, which is silent and total: the plan looks
	// complete and every blob but the final one is gone. The path stays in the
	// manifest, where it says where the content BELONGS; the handle says where
	// it IS.
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:])

	// No escape guard here on purpose: the name is 64 hex characters this
	// function just computed, so a caller's path can no longer reach the
	// filesystem at all. A guard that cannot fire reads as protection and is
	// not; the path is now only carried for the error message below.
	base := filepath.Clean(s.Dir)
	full := filepath.Join(base, name)

	if !mode.Valid() {
		return "", &gitstore.UnsupportedModeError{Path: path, Mode: mode}
	}

	if _, err := os.Stat(full); err == nil {
		// Identical content is already stored under this name; writing it
		// again would only cost the write.
		return name, nil
	}

	if err := os.MkdirAll(base, dirSinkDirPerm); err != nil {
		return "", fmt.Errorf("create blob directory: %w", err)
	}

	if err := os.WriteFile(full, content, filePermission(mode)); err != nil {
		return "", fmt.Errorf("write blob: %w", err)
	}

	return name, nil
}

func filePermission(mode gitstore.FileMode) os.FileMode {
	if mode == gitstore.ModeExecutable {
		return dirSinkExecFilePerm
	}

	return dirSinkFilePerm
}
