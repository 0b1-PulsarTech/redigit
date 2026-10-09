package dart

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// exportRule makes an export directive wait for the file it exports: for the
// names its show list passes on, or else for the file's first declaration,
// which is what makes the file exist. Waiting for the whole file instead
// would hold a barrel back until everything behind it was written.
type exportRule struct{}

func (exportRule) Name() string { return "export" }

func (exportRule) Edges(
	ctx context.Context,
	space *workspace,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	var out []entities.Edge

	for _, file := range dartFiles(decls) {
		parsed, _, err := space.file(ctx, file)
		if err != nil {
			return nil, err
		}

		for _, it := range parsed.items {
			if it.kind != itemExport {
				continue
			}

			found, exportErr := exportEdges(ctx, space, parsed.path, it)
			if exportErr != nil {
				return nil, exportErr
			}

			out = append(out, found...)
		}
	}

	return out, nil
}

func (exportRule) Anchors(context.Context, *workspace, []entities.Decl) ([]entities.Anchor, error) {
	return nil, nil
}

func exportEdges(
	ctx context.Context,
	space *workspace,
	from string,
	export parsedItem,
) ([]entities.Edge, error) {
	target, err := space.resolve(ctx, from, export.directive.uri)
	if err != nil || target == "" {
		return nil, err
	}

	if len(export.directive.show) == 0 {
		exported, _, readErr := space.file(ctx, target)
		if readErr != nil {
			return nil, readErr
		}

		return referenceEdges(export.key, provider{key: firstDecl(exported)}), nil
	}

	offered, err := space.exported(ctx, target)
	if err != nil {
		return nil, err
	}

	var out []entities.Edge

	for _, name := range export.directive.show {
		out = append(out, referenceEdges(export.key, offered.names[name])...)
	}

	return out, nil
}
