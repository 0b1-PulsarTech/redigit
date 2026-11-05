package session

import (
	"cmp"
	"flag"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/infra/plancache"
)

const (
	flagResolver = "resolver"
	flagMode     = "mode"
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
	case "repo", "rev", "tags", "server", "virtual", "resolver", "mode":
		return ClassKey, true
	case "layers", "features", "chunk-map", "config":
		return ClassFile, true
	case "far",
		"blob-dir",
		"state-dir",
		"cpuprofile",
		"memprofile",
		"trace",
		"timings",
		"mem-limit",
		"from-cache":
		return ClassRunOnly, true
	}

	return ClassKey, false
}

func Flags(set *flag.FlagSet, configBytes []byte) []plancache.Flag {
	var flags []plancache.Flag

	set.Visit(func(setFlag *flag.Flag) {
		class, _ := Classify(setFlag.Name)
		if _, isEffective := effectiveDefault(setFlag.Name); class == ClassRunOnly || isEffective {
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

	// Keyed by what the run will use: the default and the same value typed out build one graph.
	for _, name := range []string{flagResolver, flagMode} {
		if setFlag := set.Lookup(name); setFlag != nil {
			fallback, _ := effectiveDefault(name)
			flags = append(flags, plancache.Flag{
				Name: name, Value: cmp.Or(setFlag.Value.String(), fallback),
			})
		}
	}

	slices.SortFunc(flags, func(left, right plancache.Flag) int {
		return strings.Compare(left.Name, right.Name)
	})

	return flags
}

func effectiveDefault(name string) (fallback string, isEffective bool) {
	switch name {
	case flagResolver:
		return bootstrap.ResolverTyped, true
	case flagMode:
		return bootstrap.ModeHistory, true
	}

	return "", false
}
