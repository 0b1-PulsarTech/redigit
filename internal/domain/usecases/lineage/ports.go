package lineage

import (
	"context"
	"errors"
	"iter"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var ErrBlobMissing = errors.New("blob missing")

const (
	StatusDeleted byte = 'D'
	StatusRenamed byte = 'R'
)

type (
	RevisionLog interface {
		Changes(ctx context.Context) iter.Seq2[Revision, error]
		ReadBlob(ctx context.Context, oid string) ([]byte, error)
	}

	Parser interface {
		Declarations(ctx context.Context, path string, src []byte) ([]entities.Decl, error)
	}
)

type (
	Revision struct {
		Rev     string
		Changes []Change
	}

	Change struct {
		Path       string
		OldPath    string
		Blob       string
		OldBlob    string
		Status     byte
		Similarity int
	}

	Language struct {
		Extensions []string
		Parser     Parser
	}

	Config struct {
		Languages   []Language
		Concurrency int
		Follow      bool
	}
)

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=ports.go -destination=ports_mock_test.go -package=lineage
