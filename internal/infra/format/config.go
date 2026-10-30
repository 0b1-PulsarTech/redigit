package format

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type Mode string

const (
	ModeFiles Mode = "files"
	ModeStdin Mode = "stdin"
)

type Duration time.Duration

func (duration *Duration) UnmarshalJSON(raw []byte) error {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return fmt.Errorf("a duration is a string like \"30s\": %w", err)
	}

	parsed, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("parse duration: %w", err)
	}

	*duration = Duration(parsed)

	return nil
}

type Step struct {
	Cmd        []string `json:"cmd"`
	Mode       Mode     `json:"mode"`
	Timeout    Duration `json:"timeout"`
	IfWider    int      `json:"if_wider"`
	TabWidth   int      `json:"tab_width"`
	AllowEmpty bool     `json:"allow_empty"`
}

type Language struct {
	Name       string   `json:"name"`
	Extensions []string `json:"extensions"`
	SkipMarker string   `json:"skip_marker"`
	Steps      []Step   `json:"steps"`
	Isolate    []string `json:"isolate"`
}

type Config struct {
	Languages  []Language `json:"languages"`
	Workers    int        `json:"workers"`
	BatchSize  int        `json:"batch_size"`
	ConfigRoot string     `json:"config_root"`
	CopyFiles  []string   `json:"copy_files"`
}

func Decode(reader io.Reader) (Config, error) {
	var config Config

	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode format config: %w", err)
	}

	return config, nil
}

var ErrConfig = errors.New("invalid format config")

type MissingToolError struct {
	Tool string
	Err  error
}

func (missing *MissingToolError) Error() string {
	return fmt.Sprintf("formatter %q is not installed: %v", missing.Tool, missing.Err)
}

func (missing *MissingToolError) Unwrap() error { return missing.Err }

type Failure struct {
	Tool   string
	Path   string
	Stderr string
	Err    error
}

func (failure *Failure) Error() string {
	return fmt.Sprintf(
		"format %s with %s: %v: %s",
		failure.Path,
		failure.Tool,
		failure.Err,
		failure.Stderr,
	)
}

func (failure *Failure) Unwrap() error { return failure.Err }

var (
	ErrEmptyOutput    = errors.New("formatter emptied a non-empty blob")
	ErrOutputTooLarge = errors.New("formatter output is too large")
	ErrOutputKind     = errors.New("formatter left a non-regular file")
	ErrStepAborted    = errors.New("step aborted")
	errStart          = errors.New("tool did not start")
)

func anchored(cmd, root string) string {
	if filepath.IsAbs(cmd) || !strings.ContainsRune(cmd, filepath.Separator) {
		return cmd
	}

	// a relative path would otherwise resolve against each temp dir the tool runs in
	if root == "" {
		root = "."
	}

	if abs, err := filepath.Abs(filepath.Join(root, cmd)); err == nil {
		return abs
	}

	return cmd
}

func emptied(step tool, item job, out []byte) bool {
	return len(out) == 0 && len(item.content) > 0 && !step.empty
}

func checkEmpty(step tool, item job, out []byte) result {
	if emptied(step, item, out) {
		return result{failure: newFailure(step, item.path, "", ErrEmptyOutput)}
	}

	return result{content: out}
}

func newFailure(step tool, file, stderr string, err error) *Failure {
	const stderrExcerpt = 240

	excerpt := strings.TrimSpace(stderr)
	if len(excerpt) > stderrExcerpt {
		cut := stderrExcerpt
		for cut > 0 && !utf8.RuneStart(excerpt[cut]) {
			cut--
		}

		excerpt = excerpt[:cut]
	}

	return &Failure{Tool: step.name, Path: file, Stderr: excerpt, Err: err}
}
