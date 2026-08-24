package golang

import "regexp"

// generatedMarker is the line the Go project defines for a generated file, and
// the line every generator in the toolchain writes.
//
// Detecting by that line rather than by a name pattern is what tells a
// generated mock from a hand-written one: a repository that hand-rolls small
// mocks has files named _mock_test.go that a person wrote and that SHOULD be
// cut by declaration. On a real tree the marker split 228 generated files from
// 5 hand-written ones with the same suffix.
var generatedMarker = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)

// isGenerated reports whether source carries the marker.
func isGenerated(source []byte) bool { return generatedMarker.Match(source) }
