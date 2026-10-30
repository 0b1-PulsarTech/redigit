package format

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
	"unicode"
)

type tool struct {
	name    string
	argv    []string
	mode    Mode
	timeout time.Duration
	wider   int
	tabs    int
	empty   bool
}

func compileStep(step Step, root string) (tool, error) {
	const (
		defaultTimeout  = 30 * time.Second
		defaultTabWidth = 4
	)

	if len(step.Cmd) == 0 {
		return tool{}, fmt.Errorf("%w: a step has an empty cmd", ErrConfig)
	}

	if step.Mode != ModeFiles && step.Mode != ModeStdin {
		return tool{}, fmt.Errorf("%w: step %q has mode %q", ErrConfig, step.Cmd[0], step.Mode)
	}

	resolved, err := exec.LookPath(anchored(step.Cmd[0], root))
	if err != nil {
		return tool{}, &MissingToolError{Tool: step.Cmd[0], Err: err}
	}

	timeout := time.Duration(step.Timeout)
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	argv := append([]string{resolved}, step.Cmd[1:]...)

	tabs := step.TabWidth
	if tabs <= 0 {
		tabs = defaultTabWidth
	}

	return tool{
		name: filepath.Base(step.Cmd[0]), argv: argv, mode: step.Mode, timeout: timeout,
		wider: step.IfWider, tabs: tabs, empty: step.AllowEmpty,
	}, nil
}

func (step tool) needs(content []byte) bool {
	if step.wider <= 0 {
		return true
	}

	for line := range bytes.Lines(content) {
		if columns(bytes.TrimRight(line, "\r\n"), step.tabs) > step.wider {
			return true
		}
	}

	return false
}

func columns(line []byte, tabs int) int {
	width := 0

	for _, char := range string(line) {
		if char == '\t' {
			width += tabs - width%tabs

			continue
		}

		width += runeWidth(char)
	}

	return width
}

func runeWidth(char rune) int {
	const (
		firstCombining = 0x300
		wideWidth      = 2
	)

	switch {
	case char < firstCombining:
		return 1
	case isZeroWidth(char):
		return 0
	case isWide(char):
		return wideWidth
	default:
		return 1
	}
}

func isZeroWidth(char rune) bool {
	// ZWJ and the variation selectors restyle the glyph before them; medial and final Jamo join it
	return char == 0x200D || char == 0xFE0E || char == 0xFE0F || char >= 0x1160 && char <= 0x11FF ||
		unicode.In(char, unicode.Mn, unicode.Me)
}

func isWide(char rune) bool {
	// stdlib has no East Asian width table: common wide blocks and emoji presentation only
	wide := [...][2]rune{
		{0x1100, 0x115F},
		{0x231A, 0x231B},
		{0x23E9, 0x23EC},
		{0x25FD, 0x25FE},
		{0x2614, 0x2615},
		{0x2648, 0x2653},
		{0x267F, 0x267F},
		{0x2693, 0x2693},
		{0x26A1, 0x26A1},
		{0x26AA, 0x26AB},
		{0x26BD, 0x26BE},
		{0x26C4, 0x26C5},
		{0x26CE, 0x26CE},
		{0x26D4, 0x26D4},
		{0x26EA, 0x26EA},
		{0x26F2, 0x26F3},
		{0x26F5, 0x26F5},
		{0x26FA, 0x26FA},
		{0x26FD, 0x26FD},
		{0x2705, 0x2705},
		{0x2728, 0x2728},
		{0x274C, 0x274C},
		{0x2753, 0x2755},
		{0x2757, 0x2757},
		{0x2795, 0x2797},
		{0x27B0, 0x27B0},
		{0x27BF, 0x27BF},
		{0x2B1B, 0x2B1C},
		{0x2B50, 0x2B50},
		{0x2B55, 0x2B55},
		// CJK radicals to Yi, minus U+303F and the Yijing hexagrams U+4DC0-4DFF
		{0x2E80, 0x303E},
		{0x3040, 0x4DBF},
		{0x4E00, 0xA4CF},
		{0xAC00, 0xD7A3},
		{0xF900, 0xFAFF},
		{0xFE30, 0xFE6F},
		{0xFF00, 0xFF60},
		{0xFFE0, 0xFFE6},
		{0x1F004, 0x1F004},
		{0x1F0CF, 0x1F0CF},
		{0x1F18E, 0x1F18E},
		{0x1F191, 0x1F19A},
		{0x1F200, 0x1F251},
		{0x1F300, 0x1F64F},
		{0x1F680, 0x1F6FF},
		{0x1F7E0, 0x1F7EB},
		{0x1F900, 0x1F9FF},
		{0x1FA70, 0x1FAFF},
		{0x20000, 0x3FFFD},
	}

	for _, span := range wide {
		if char >= span[0] && char <= span[1] {
			return true
		}
	}

	return false
}
