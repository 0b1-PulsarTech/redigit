package anchorstate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
)

const maxName = 120

func stemOf(idx int, name string) string {
	const hashSize = 8

	name = strings.Map(func(char rune) rune {
		if strings.ContainsRune("/\\ \r\n", char) {
			return '_'
		}

		return char
	}, name)

	if len(name) > maxName {
		sum := sha256.Sum256([]byte(name))
		cut := maxName - hashSize - 1

		// A cut inside a character would leave a name that is not valid text.
		for cut > 0 && !utf8.RuneStart(name[cut]) {
			cut--
		}

		name = name[:cut] + "-" + hex.EncodeToString(sum[:])[:hashSize]
	}

	return fmt.Sprintf("%02d-%s", idx, name)
}

func writeConflict(dir string, conflict anchoring.Conflict) error {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("make the conflict directory: %w", err)
	}

	for idx, decl := range conflict.Decls {
		stem := filepath.Join(dir, stemOf(idx, decl.Key.Name))

		// No fsync per text: state.json comes last, so a kill leaves no state beside a torn text.
		// Power loss can still tear one beside a durable state.json; continue never reads the
		// texts, which are for the user. Kill-safe, not power-safe.
		err := errors.Join(
			os.WriteFile(stem+".preimage", []byte(decl.Preimage), fileMode),
			os.WriteFile(stem+".final", []byte(decl.Final), fileMode),
		)
		if err != nil {
			return fmt.Errorf("write a conflict text: %w", err)
		}
	}

	return nil
}

func writeTemp(dir string, content []byte) (name string, err error) {
	var staged *os.File

	staged, err = os.CreateTemp(dir, tmpPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("create a temporary file: %w", err)
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, ignoreMissing(os.Remove(staged.Name())))
		}
	}()

	if _, err = staged.Write(content); err != nil {
		return "", errors.Join(fmt.Errorf("write a temporary file: %w", err), staged.Close())
	}

	if err = errors.Join(staged.Sync(), staged.Close()); err != nil {
		return "", fmt.Errorf("flush a temporary file: %w", err)
	}

	return staged.Name(), nil
}

func listing(conflict anchoring.Conflict) []string {
	names := make([]string, 0, len(conflict.Decls))

	for idx, decl := range conflict.Decls {
		stem := conflict.Rev + "/" + stemOf(idx, decl.Key.Name)
		names = append(names, stem+".preimage", stem+".final")
	}

	return names
}
