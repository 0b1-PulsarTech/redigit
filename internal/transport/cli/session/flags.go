package session

import (
	"flag"

	"github.com/wrapped-owls/redigit/internal/infra/plancache"
)

type FlagClass int

const (
	ClassKey FlagClass = iota
	ClassFile
	ClassRunOnly
)

func Classify(name string) (class FlagClass, isKnown bool) {
	// An unknown flag is a key flag, the safe default; a test fails until named.
	switch name {
	case "repo", "rev", "tags", "server", "virtual":
		return ClassKey, true
	case "layers", "features", "chunk-map", "config":
		return ClassFile, true
	case "far", "blob-dir", "cpuprofile", "memprofile", "trace", "timings", "mem-limit",
		"from-cache":
		return ClassRunOnly, true
	}

	return ClassKey, false
}

func Flags(set *flag.FlagSet, configBytes []byte) []plancache.Flag {
	var flags []plancache.Flag

	set.Visit(func(setFlag *flag.Flag) {
		class, _ := Classify(setFlag.Name)
		if class == ClassRunOnly {
			return
		}

		flag := plancache.Flag{
			Name: setFlag.Name, Value: setFlag.Value.String(), IsFile: class == ClassFile,
		}

		// The config was already read once; keying the path again could see another file.
		if setFlag.Name == "config" {
			flag.Content = append([]byte{}, configBytes...)
		}

		flags = append(flags, flag)
	})

	return flags
}
