//go:build unix

package main

import "syscall"

func plantFifo(name string) error { return syscall.Mkfifo(name, 0o600) }
