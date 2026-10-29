package main

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

func main() {
	dir := os.Getenv("FAKEGIT_DIR")
	text, _ := strconv.Unquote(os.Getenv("FAKEGIT_TEXT"))

	switch os.Getenv("FAKEGIT_MODE") {
	case "linger":
		linger(dir)
	case "fail":
		_, _ = os.Stdout.WriteString(text)
		_, _ = os.Stderr.WriteString("fatal: boom\n")
		os.Exit(1)
	case "reply":
		reply(text)
	case "oversize":
		oversize()
	case "silent":
		silent(dir)
	case "exit0":
		_, _ = os.Stdout.WriteString(text)
	case "flood":
		flood(dir, text)
	case "midrecord":
		midrecord(dir, text)
	}
}

func linger(dir string) {
	_ = os.WriteFile(filepath.Join(dir, "pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
	_ = os.Stdout.Close()

	gate, err := os.Open(filepath.Join(dir, "gate"))
	if err != nil {
		os.Exit(2)
	}

	_, _ = io.Copy(io.Discard, gate)
	_ = os.WriteFile(filepath.Join(dir, "done"), nil, 0o600)
}

func reply(text string) {
	input := bufio.NewReader(os.Stdin)

	for {
		if _, err := input.ReadString('\n'); err != nil {
			return
		}

		_, _ = os.Stdout.WriteString(text)
	}
}

func silent(dir string) {
	input := bufio.NewReader(os.Stdin)
	if _, err := input.ReadString('\n'); err != nil {
		return
	}

	ready, err := os.OpenFile(filepath.Join(dir, "ready"), os.O_WRONLY, 0)
	if err != nil {
		os.Exit(2)
	}

	_ = ready.Close()

	_, _ = io.Copy(io.Discard, input)
}

func oversize() {
	const size = 256<<20 + 1

	input := bufio.NewReader(os.Stdin)
	output := bufio.NewWriter(os.Stdout)

	isFirst := true

	for {
		if _, err := input.ReadString('\n'); err != nil {
			return
		}

		if !isFirst {
			_, _ = output.WriteString("abc blob 3\nabc\n")
			_ = output.Flush()

			continue
		}

		isFirst = false

		_, _ = output.WriteString("big blob " + strconv.Itoa(size) + "\n")
		_, _ = io.CopyN(output, zeroes{}, size)
		_, _ = output.WriteString("\n")
		_ = output.Flush()
	}
}

type zeroes struct{}

func (zeroes) Read(buf []byte) (int, error) { return len(buf), nil }

func flood(dir, text string) {
	_ = os.WriteFile(filepath.Join(dir, "pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
	_, _ = os.Stdout.WriteString(text)

	// Far more than a pipe holds, so git stays alive blocked on the write.
	_, _ = io.CopyN(os.Stdout, zeroes{}, 1<<20)
}

func midrecord(dir, text string) {
	_, _ = os.Stdout.WriteString(text)

	ready, err := os.OpenFile(filepath.Join(dir, "ready"), os.O_WRONLY, 0)
	if err != nil {
		os.Exit(2)
	}

	_ = ready.Close()

	// Nothing ever opens the gate for writing, so git waits here until killed.
	gate, err := os.Open(filepath.Join(dir, "gate"))
	if err != nil {
		os.Exit(2)
	}

	_, _ = io.Copy(io.Discard, gate)
}
