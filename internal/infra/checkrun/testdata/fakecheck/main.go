package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

const backstop = 90 * time.Second

func main() {
	if len(os.Args) < 2 {
		os.Exit(64)
	}

	mode, rest := os.Args[1], os.Args[2:]

	switch mode {
	case "pass":
		logLine(rest, "pass")
	case "fail":
		logLine(rest, "fail")
		fmt.Fprint(os.Stderr, strings.Join(rest[min(1, len(rest)):], " "))
		os.Exit(1)
	case "crash":
		logLine(rest, "crash")
		fmt.Fprint(os.Stderr, strings.Join(rest[min(1, len(rest)):], " "))
		os.Exit(2)
	case "flood":
		for line := range 4000 {
			fmt.Fprintf(os.Stderr, "line %04d of the flood\n", line)
		}

		os.Exit(1)
	case "hang":
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt)

		// Backstop: a hang that nobody killed must not outlive the test run.
		select {
		case <-stop:
		case <-time.After(backstop):
		}
	case "env":
		dumpEnv(rest)
	case "orphan":
		leaveOrphan(rest)
	case "version":
		logLine(rest, "version")
		fmt.Println("fakecheck 1.0")
	case "brokenversion":
		for line := range 60 {
			fmt.Fprintf(os.Stderr, "noise %d\n", line)
		}

		fmt.Fprintln(os.Stderr, "cache program exploded")
		os.Exit(3)
	}
}

func logLine(args []string, what string) {
	if len(args) == 0 {
		return
	}

	cwd, _ := os.Getwd()

	handle, err := os.OpenFile(args[0], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}

	fmt.Fprintf(handle, "%s %s\n", what, cwd)
	handle.Close()
}

func dumpEnv(args []string) {
	work, isSet := os.LookupEnv("GOWORK")
	body := fmt.Sprintf("GOWORK=%s\nGOWORK_SET=%t\n", work, isSet)
	body += "GO111MODULE=" + os.Getenv("GO111MODULE") + "\nGOFLAGS=" + os.Getenv("GOFLAGS") + "\n"

	for _, entry := range os.Environ() {
		body += entry + "\n"
	}

	cwd, _ := os.Getwd()
	_ = os.WriteFile(args[0], []byte(body+"cwd="+cwd+"\n"), 0o600)
}

func leaveOrphan(args []string) {
	child := exec.Command(os.Args[0], "hang")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr

	if err := child.Start(); err != nil {
		os.Exit(70)
	}

	_ = os.WriteFile(args[0], []byte(strconv.Itoa(child.Process.Pid)), 0o600)
}
