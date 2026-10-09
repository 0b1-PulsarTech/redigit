package dart

import "context"

func (w *workspace) declareWithParts(
	ctx context.Context,
	f parsedFile,
	out namespace,
	isPrivateKept bool,
) error {
	out.declare(f, isPrivateKept)

	for _, it := range f.items {
		if it.kind != itemPart {
			continue
		}

		target, err := w.resolve(ctx, f.path, it.directive.uri)
		if err != nil {
			return err
		}

		part, isOwned, err := w.file(ctx, target)
		if err != nil {
			return err
		}

		if isOwned {
			out.declare(part, isPrivateKept)
		}
	}

	return nil
}
