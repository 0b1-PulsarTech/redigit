package fixture

import "io"

type ReaderA struct{}

func (ReaderA) Read(p []byte) (int, error) { return 0, nil }

type ReaderB struct{}

func (ReaderB) Read(p []byte) (int, error) { return 0, nil }

var _ io.Reader = (*ReaderA)(nil)

var _ io.Reader = (*ReaderB)(nil)
