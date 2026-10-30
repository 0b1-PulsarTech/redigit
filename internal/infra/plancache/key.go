package plancache

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"maps"
	"slices"
	"strings"
)

const formatVersion = 2

type Flag struct {
	Name   string
	Value  string
	IsFile bool
	// The parsed bytes of a file flag; when set the key never opens Value.
	Content []byte
}

type Inputs struct {
	BuildID    string
	Commit     string
	Server     string
	ServerArgs []string
	Toolchain  string
	Virtual    map[string]string
	Flags      []Flag
}

func (inputs Inputs) Key() (string, error) {
	// Every field is length-prefixed, so two inputs never run together into one.
	hasher := sha256.New()

	put(hasher, "format", string(binary.AppendUvarint(nil, formatVersion)))
	put(hasher, "build", inputs.BuildID)
	put(hasher, "commit", inputs.Commit)
	put(hasher, "server", inputs.Server)
	put(hasher, "server-args", strings.Join(inputs.ServerArgs, "\x00"))
	put(hasher, "toolchain", inputs.Toolchain)

	for _, id := range slices.Sorted(maps.Keys(inputs.Virtual)) {
		put(hasher, "virtual", id, inputs.Virtual[id])
	}

	flags := slices.Clone(inputs.Flags)
	slices.SortFunc(
		flags,
		func(left, right Flag) int { return strings.Compare(left.Name, right.Name) },
	)

	for _, flag := range flags {
		value := flag.Value
		kind := "value"

		if flag.IsFile {
			kind = "file"
			value = ""

			if flag.Content != nil {
				value = BytesDigest(flag.Content)
			} else if flag.Value != "" {
				digest, err := FileDigest(flag.Value)
				if err != nil {
					return "", err
				}

				value = digest
			}
		}

		put(hasher, "flag", flag.Name, kind, value)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func put(hasher hash.Hash, fields ...string) {
	for _, field := range fields {
		_, _ = hasher.Write(binary.AppendUvarint(nil, uint64(len(field))))
		_, _ = hasher.Write([]byte(field))
	}
}
