package lsp

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"unicode/utf16"
	"unicode/utf8"
)

// ErrOffsetOutOfRange reports an offset that does not land inside line, or
// (for a byte offset) not on a UTF-8 rune boundary within it.
var ErrOffsetOutOfRange = errors.New("lsp: offset out of range")

// ErrInvalidUTF8 reports a line that is not valid UTF-8, which neither
// conversion can meaningfully address.
var ErrInvalidUTF8 = errors.New("lsp: invalid utf-8")

// ByteToUTF16 converts a byte offset within line (a single line's bytes, no
// trailing newline expected) to the UTF-16 code-unit offset the LSP wire
// format uses for Position.Character. gopls never advertises
// capabilities.positionEncoding and always operates in UTF-16 regardless of
// what a client offers, so every Position this package sends or reads for
// gopls needs this conversion applied by the caller holding the byte
// offset. A character outside the Basic Multilingual Plane (an emoji is the
// common case) is a surrogate pair and counts as 2 UTF-16 units for 1 rune.
func ByteToUTF16(line []byte, byteOffset int) (int, error) {
	if byteOffset < 0 || byteOffset > len(line) {
		return 0, fmt.Errorf("byte to utf16: %w", ErrOffsetOutOfRange)
	}

	units := 0

	i := 0
	for i < byteOffset {
		r, size := utf8.DecodeRune(line[i:])
		if r == utf8.RuneError && size <= 1 {
			return 0, fmt.Errorf("byte to utf16: %w", ErrInvalidUTF8)
		}

		units += utf16.RuneLen(r)
		i += size
	}

	if i != byteOffset {
		// byteOffset landed inside a multi-byte rune instead of on its
		// boundary; the loop above overshot past it decoding that rune.
		return 0, fmt.Errorf("byte to utf16: %w", ErrOffsetOutOfRange)
	}

	return units, nil
}

// UTF16ToByte is the inverse of ByteToUTF16: it converts a UTF-16 code-unit
// offset within line back to a byte offset.
func UTF16ToByte(line []byte, utf16Offset int) (int, error) {
	if utf16Offset < 0 {
		return 0, fmt.Errorf("utf16 to byte: %w", ErrOffsetOutOfRange)
	}

	units := 0
	i := 0

	for i < len(line) {
		if units == utf16Offset {
			return i, nil
		}

		r, size := utf8.DecodeRune(line[i:])
		if r == utf8.RuneError && size <= 1 {
			return 0, fmt.Errorf("utf16 to byte: %w", ErrInvalidUTF8)
		}

		units += utf16.RuneLen(r)
		i += size
	}

	if units == utf16Offset {
		return i, nil
	}

	return 0, fmt.Errorf("utf16 to byte: %w", ErrOffsetOutOfRange)
}

// ErrRelativePath reports a path that cannot become a file:// URI, since
// that scheme has no notion of "relative to the server's cwd".
var ErrRelativePath = errors.New("lsp: path must be absolute")

// ErrUnsupportedScheme reports a URI this client cannot map back to a
// filesystem path, because it did not come from the file:// scheme this
// client always sends.
var ErrUnsupportedScheme = errors.New("lsp: unsupported uri scheme")

// pathToURI turns an absolute filesystem path into the file:// URI form
// every request in this subset addresses a document by.
func pathToURI(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path to uri: %w", ErrRelativePath)
	}

	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}

	return u.String(), nil
}

// uriToPath is the inverse of pathToURI, used to turn a server-returned
// Location.URI back into the plain path entities.Location carries.
func uriToPath(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("uri to path: %w", err)
	}

	if parsed.Scheme != "file" {
		return "", fmt.Errorf("uri to path: %w", ErrUnsupportedScheme)
	}

	return filepath.FromSlash(parsed.Path), nil
}
