package lineindex

import (
	"unicode/utf16"
	"unicode/utf8"
)

func runeSizeAt(source []byte, offset int) int {
	_, size := utf8.DecodeRune(source[offset:])
	if size == 0 {
		return 1
	}

	return size
}

func utf16Units(encoded []byte) int {
	return len(utf16.Encode([]rune(string(encoded))))
}
