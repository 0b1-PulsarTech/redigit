package dart

import (
	"context"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// testAnchorRule pulls a test's main toward what it exercises. A Dart package
// mirrors lib/src/<path>.dart in test/<path>_test.dart, so the mirrored file
// names the subject's file and the first of its declarations the test reads
// names the subject; with no mirror, the first library declaration it reads.
type testAnchorRule struct{}

func (testAnchorRule) Name() string { return "test-anchor" }

func (testAnchorRule) Edges(context.Context, *workspace, []entities.Decl) ([]entities.Edge, error) {
	return nil, nil
}

func (testAnchorRule) Anchors(
	ctx context.Context,
	space *workspace,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	var out []entities.Anchor

	for _, decl := range decls {
		if !decl.IsTest || path.Ext(decl.Key.File) != dartExt {
			continue
		}

		subject, found, err := testSubject(ctx, space, decl.Key)
		if err != nil {
			return nil, err
		}

		if found {
			out = append(
				out,
				entities.Anchor{Decl: decl.Key, To: subject, Reason: entities.ReasonTestOf},
			)
		}
	}

	return out, nil
}

func testSubject(
	ctx context.Context,
	space *workspace,
	test entities.DeclKey,
) (entities.DeclKey, bool, error) {
	parsed, _, err := space.file(ctx, test.File)
	if err != nil {
		return entities.DeclKey{}, false, err
	}

	visible, err := space.scopeOf(ctx, parsed)
	if err != nil {
		return entities.DeclKey{}, false, err
	}

	var reads []entities.DeclKey

	for _, it := range parsed.decls() {
		if it.key == test {
			for _, ref := range visible.references(it.tokens(parsed.toks), it.key) {
				reads = append(reads, ref.key)
			}
		}
	}

	for _, mirror := range mirrors(test.File) {
		if subject, found := firstIn(
			reads,
			func(key entities.DeclKey) bool { return key.File == mirror },
		); found {
			return subject, true, nil
		}

		if mirrored, isOwned, readErr := space.file(ctx, mirror); readErr != nil || isOwned {
			return firstDecl(mirrored), isOwned, readErr
		}
	}

	subject, found := firstIn(reads, func(key entities.DeclKey) bool {
		return !key.IsZero() && !isTestPath(key.File)
	})

	return subject, found, nil
}

// mirrors names the files a test file mirrors: lib/src/<path>.dart first,
// then lib/<path>.dart, for test/<path>_test.dart in the same package.
func mirrors(testFile string) []string {
	dir, rest, found := cutTestDir(testFile)
	if !found || !strings.HasSuffix(rest, "_test.dart") {
		return nil
	}

	stem := strings.TrimSuffix(rest, "_test.dart") + ".dart"

	return []string{path.Join(dir, "lib", "src", stem), path.Join(dir, "lib", stem)}
}

// cutTestDir splits at the last "test" directory: what precedes it is the
// package, what follows it the mirrored path.
func cutTestDir(file string) (pkg, rest string, isFound bool) {
	if after, found := strings.CutPrefix(file, "test/"); found {
		return ".", after, true
	}

	cut := strings.LastIndex(file, "/test/")
	if cut < 0 {
		return "", "", false
	}

	return file[:cut], file[cut+len("/test/"):], true
}

func isTestPath(file string) bool {
	_, _, isUnderTest := cutTestDir(file)

	return isUnderTest || strings.HasSuffix(file, "_test.dart")
}

func firstIn(
	keys []entities.DeclKey,
	isWanted func(entities.DeclKey) bool,
) (entities.DeclKey, bool) {
	for _, key := range keys {
		if isWanted(key) {
			return key, true
		}
	}

	return entities.DeclKey{}, false
}
