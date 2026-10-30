//go:build !unix

package checkrun

import "os/exec"

func isolate(*exec.Cmd) {
	// no process groups here: the default Cancel kills only the tool itself
}

func reap(*exec.Cmd) {
	// nothing groups the children here, so there is nothing to sweep
}
