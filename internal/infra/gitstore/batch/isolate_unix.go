//go:build unix

package batch

import (
	"os/exec"
	"syscall"
)

func Isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
