package anchoring

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type (
	Delegate interface {
		Plan(graph entities.Graph, order entities.Ordering) (entities.Plan, error)
	}

	Revisions interface {
		Revision(ctx context.Context, rev string) (Revision, error)
		Blob(ctx context.Context, oid string) ([]byte, error)
	}

	Finals interface {
		Read(ctx context.Context, path string) ([]byte, error)
	}

	Reader interface {
		Reads(file, text string) ([]string, bool)
	}

	Importer interface {
		Needed(source []byte, text string) []string
	}

	Probe interface {
		Builds(ctx context.Context, edits []Edit) (Verdict, error)
	}
)

type (
	Revision struct {
		Subject string
		Files   []FileChange
	}

	FileChange struct {
		Path, OldPath, Blob, OldBlob string
	}

	Edit struct {
		File    string
		Span    entities.Span
		Text    string
		Imports []string
		Keep    []entities.DeclKey
	}

	Verdict struct {
		IsBuilt     bool
		IsUnchecked bool
		IsUnfit     bool
		Detail      string
	}
)

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=ports.go -destination=ports_mock_test.go -package=anchoring
