package verifying

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

func Key(unit Unit, identity string) string {
	digest := sha256.New()

	// Length prefixes keep "ab"+"c" and "a"+"bc" from hashing alike.
	put := func(fields ...string) {
		for _, field := range fields {
			_, _ = digest.Write([]byte(strconv.Itoa(len(field)) + ":" + field))
		}
	}

	put(identity, unit.Dir, unit.Path, unit.Language, unit.Subtree, unit.Workspace)

	for _, dep := range unit.Deps {
		put(dep.Dir, dep.Path, dep.Subtree)
	}

	return hex.EncodeToString(digest.Sum(nil))
}
