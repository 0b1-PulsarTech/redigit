package report

import "io"

// failWriter always fails, so a test can prove a command's output-write
// error path without a real broken pipe.
type failWriter struct{ err error }

func (w failWriter) Write([]byte) (int, error) { return 0, w.err }

var _ io.Writer = failWriter{}
