//go:build !unix

package main

import "errors"

func plantFifo(string) error { return errors.New("fifo is unix only") }
