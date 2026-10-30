// fakefmt upper-cases what it is given. A blob holding BAD makes it exit 1
// after finishing the others, the way gofumpt -w does; HANG makes it sleep; VANISH empties a
// blob only when the run holds more than one file.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

var (
	fifo      *os.File
	lowerMode bool
	multi     bool
)

func main() {
	if path := os.Getenv("FAKEFMT_FIFO"); path != "" {
		// opening blocks until the test reads, so it doubles as a "started" signal
		fifo, _ = os.OpenFile(path, os.O_WRONLY, 0)
	}

	if log := os.Getenv("FAKEFMT_LOG"); log != "" {
		handle, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintf(handle, "%v\n", os.Args[1:])
			handle.Close()
		}
	}

	if os.Getenv("FAKEFMT_SELFDESTRUCT") != "" {
		// a running binary may be unlinked: every later start of the tool fails
		_ = os.Remove(os.Args[0])
	}

	if os.Getenv("FAKEFMT_HANG") != "" {
		time.Sleep(time.Minute)

		return
	}

	if os.Getenv("FAKEFMT_DOWN") != "" {
		fmt.Fprintln(os.Stderr, "tool down")
		os.Exit(3)
	}

	files := os.Args[1:]
	if len(files) > 0 && files[0] == "-w" {
		files = files[1:]
	}

	if len(files) == 0 {
		content, _ := io.ReadAll(os.Stdin)

		if bytes.Contains(content, []byte("PWD")) {
			cwd, _ := os.Getwd()
			fmt.Fprint(os.Stdout, filepath.Base(cwd))

			return
		}

		out, ok := transform(content)

		if !ok {
			fmt.Fprintln(os.Stderr, "bad input")
			os.Exit(1)
		}

		os.Stdout.Write(out)

		return
	}

	if limit, _ := strconv.Atoi(os.Getenv("FAKEFMT_MAXFILES")); limit > 0 && len(files) > limit {
		time.Sleep(time.Minute)
	}

	config, _ := os.ReadFile("fake.cfg")
	lowerMode = bytes.Contains(config, []byte("LOWER"))
	multi = len(files) > 1

	failed := false

	for _, name := range files {
		if os.Getenv("FAKEFMT_PLANTFIFO") != "" {
			_ = os.Remove(name)
			_ = plantFifo(name)

			continue
		}

		if link := os.Getenv("FAKEFMT_TARGET"); link != "" {
			content, _ := os.ReadFile(name)
			if bytes.Contains(content, []byte("LINK")) {
				_ = os.Remove(name)
				_ = os.Symlink(link, name)

				continue
			}
		}

		content, err := os.ReadFile(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)

			failed = true

			continue
		}

		out, ok := transform(content)
		if !ok {
			switch {
			case os.Getenv("FAKEFMT_SILENT") != "":
				// no stderr at all: the exit status is the only evidence
			case os.Getenv("FAKEFMT_ANON") != "":
				fmt.Fprintln(os.Stderr, "bad input")
			default:
				fmt.Fprintf(os.Stderr, "%s: bad input\n", name)
			}

			failed = true

			continue
		}

		_ = os.WriteFile(name, out, 0o600)
	}

	if failed {
		os.Exit(1)
	}
}

func transform(content []byte) ([]byte, bool) {
	if bytes.Contains(content, []byte("SPAWN")) {
		spawnGrandchild()
	}

	if bytes.Contains(content, []byte("HANG")) {
		time.Sleep(time.Minute)
	}

	if bytes.Contains(content, []byte("BAD")) {
		return nil, false
	}

	if bytes.Contains(content, []byte("EMPTY")) || multi && bytes.Contains(content, []byte("VANISH")) {
		return nil, true
	}

	if bytes.Contains(content, []byte("HUGE")) {
		return bytes.Repeat([]byte("x"), 9<<20), true
	}

	if bytes.Contains(content, []byte("BGCHILD")) {
		spawnBackground()
	}

	if lowerMode {
		return bytes.ToLower(content), true
	}

	return bytes.ToUpper(content), true
}

// the grandchild shares the tool's output pipes, so only a group kill ends it
func spawnGrandchild() {
	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), "FAKEFMT_HANG=1", "FAKEFMT_FIFO=")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr

	if fifo != nil {
		child.ExtraFiles = []*os.File{fifo}
	}

	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	_ = os.WriteFile(os.Getenv("FAKEFMT_PIDFILE"), []byte(strconv.Itoa(child.Process.Pid)), 0o600)

	time.Sleep(time.Minute)
}

// the background child holds only the fifo, so the tool itself returns promptly
func spawnBackground() {
	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), "FAKEFMT_HANG=1", "FAKEFMT_FIFO=")

	if fifo != nil {
		child.ExtraFiles = []*os.File{fifo}
	}

	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
