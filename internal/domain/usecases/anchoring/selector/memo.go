package selector

import (
	"slices"
	"strings"
	"sync"
)

type (
	Reader interface {
		Reads(file, text string) ([]string, bool)
	}

	Memo struct {
		reader Reader
		mutex  sync.Mutex
		seen   map[readKey]readResult
	}

	readKey struct{ file, text string }

	readResult struct {
		names    []string
		isParsed bool
	}
)

func NewMemo(reader Reader) *Memo {
	return &Memo{reader: reader, seen: map[readKey]readResult{}}
}

func (memo *Memo) Removed(file, preimage, final, method string, imports []string) bool {
	names, isParsed := memo.reads(file, preimage)
	if !isParsed {
		// An unparsed pre-image keeps the text search, for both texts.
		return removedByText(preimage, final, method, imports)
	}

	if !selects(names, method, locals(imports)) {
		return false
	}

	// A final text that does not parse holds nothing, so the anchor folds.
	held, isFinalParsed := memo.reads(file, final)

	return !isFinalParsed || !slices.Contains(held, "."+method)
}

func (memo *Memo) reads(file, text string) ([]string, bool) {
	if memo == nil || memo.reader == nil {
		return nil, false
	}

	memo.mutex.Lock()
	defer memo.mutex.Unlock()

	key := readKey{file: file, text: text}
	if result, isSeen := memo.seen[key]; isSeen {
		return result.names, result.isParsed
	}

	names, isParsed := memo.reader.Reads(file, text)
	memo.seen[key] = readResult{names: names, isParsed: isParsed}

	return names, isParsed
}

func selects(names []string, method string, packages []string) bool {
	suffix := "." + method

	return slices.ContainsFunc(names, func(name string) bool {
		qualifier, isSelector := strings.CutSuffix(name, suffix)

		return isSelector && !slices.Contains(packages, qualifier)
	})
}
