package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
)

// sinkAdapter carries a blob store behind the emitter's Sink, which knows only
// that it hands over content and gets a handle back. The path travels because a
// directory store addresses by path while an object database addresses by
// content, and the emitter knows neither.
type sinkAdapter struct {
	put func(ctx context.Context, path string, mode gitstore.FileMode, content []byte) (string, error)
}

func (s sinkAdapter) Write(
	ctx context.Context,
	path string,
	content []byte,
	mode emitting.FileMode,
) (emitting.Handle, error) {
	handle, err := s.put(ctx, path, gitstore.FileMode(mode), content)
	if err != nil {
		return "", fmt.Errorf("store blob: %w", err)
	}

	return emitting.Handle(handle), nil
}

type chunkDTO struct {
	Key  string `json:"key"`
	Span [2]int `json:"span"`
	Unit string `json:"unit"`
}

type blobDTO struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Handle string `json:"handle"`
}

type riskDTO struct {
	Chunk  string `json:"chunk"`
	Target string `json:"target"`
	BornAt int    `json:"born_at"`
}

// commitDTO is one line of the manifest. Every list is present even when
// empty, so a consumer never has to tell null from absent.
type manifestDTO struct {
	Position int        `json:"position"`
	Subject  string     `json:"subject"`
	Shape    string     `json:"shape"`
	Feature  string     `json:"feature,omitempty"`
	Reason   string     `json:"reason,omitempty"`
	IsWIP    bool       `json:"is_wip"`
	Chunks   []chunkDTO `json:"chunks"`
	Blobs    []blobDTO  `json:"blobs"`
	Risks    []riskDTO  `json:"risks"`
}

func runEmit(ctx context.Context, cfg bootstrap.Config, start starter, out io.Writer) error {
	return withPipeline(ctx, cfg, start, func(pl pipeline) error {
		graph, order, err := settled(ctx, pl)
		if err != nil {
			return err
		}

		plan, err := pl.Plan(graph, order)
		if err != nil {
			return fmt.Errorf("build plan: %w", err)
		}

		put, closeSink := openSink(cfg)
		defer closeSink()

		commits, err := pl.Emit(ctx, plan, graph, sinkAdapter{put: put})
		if err != nil {
			return fmt.Errorf("materialise plan: %w", err)
		}

		return writeManifest(out, commits)
	})
}

func writeManifest(out io.Writer, commits []emitting.Commit) error {
	encoder := json.NewEncoder(out)

	for _, commit := range commits {
		if err := encoder.Encode(toCommitDTO(commit)); err != nil {
			return fmt.Errorf("write manifest line: %w", err)
		}
	}

	return nil
}

func toCommitDTO(commit emitting.Commit) manifestDTO {
	chunks := make([]chunkDTO, 0, len(commit.Chunks))
	for _, chunk := range commit.Chunks {
		chunks = append(chunks, chunkDTO{
			Key:  chunk.Key.String(),
			Span: [2]int{chunk.Span.Start, chunk.Span.End},
			Unit: string(chunk.Unit),
		})
	}

	blobs := make([]blobDTO, 0, len(commit.Blobs))
	for _, blob := range commit.Blobs {
		blobs = append(blobs, blobDTO{
			Path:   blob.Path,
			Mode:   string(blob.Mode),
			Handle: string(blob.Handle),
		})
	}

	risks := make([]riskDTO, 0, len(commit.Risks))
	for _, risk := range commit.Risks {
		risks = append(risks, riskDTO{
			Chunk:  risk.Chunk.String(),
			Target: risk.Target.String(),
			BornAt: int(risk.BornAt),
		})
	}

	return manifestDTO{
		Position: int(commit.Position),
		Subject:  commit.Subject,
		Shape:    string(commit.Shape),
		Feature:  commit.Feature,
		Reason:   commit.Reason,
		IsWIP:    commit.IsWIP,
		Chunks:   chunks,
		Blobs:    blobs,
		Risks:    risks,
	}
}

// putter is the one operation a blob store offers this command.
type putter func(
	ctx context.Context,
	path string,
	mode gitstore.FileMode,
	content []byte,
) (string, error)

// openSink picks the store at the composition edge: the repository's own object
// database, or a plain directory. A directory costs duplication and buys the
// guarantee that nothing is written into a repository somebody else is using.
func openSink(cfg bootstrap.Config) (putter, func()) {
	if cfg.BlobDir != "" {
		sink := gitstore.NewDirSink(cfg.BlobDir)

		return sink.Put, func() {}
	}

	store := gitstore.NewBlobStore(cfg.Repo)

	return store.Put, func() { _ = store.Close() }
}
