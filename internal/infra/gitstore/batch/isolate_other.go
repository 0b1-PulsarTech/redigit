//go:build !unix

package batch

import "os/exec"

func Isolate(*exec.Cmd) {
	// no process groups here: the process keeps the console's group
}
