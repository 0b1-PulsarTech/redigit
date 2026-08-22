package lsp

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
)

// ErrRelativePath reports a path that cannot become a file:// URI, since
// that scheme has no notion of "relative to the server's cwd".
var ErrRelativePath = errors.New("lsp: path must be absolute")

// ErrUnsupportedScheme reports a URI this client cannot map back to a
// filesystem path, because it did not come from the file:// scheme this
// client always sends.
var ErrUnsupportedScheme = errors.New("lsp: unsupported uri scheme")

// pathToURI turns an absolute filesystem path into the file:// URI form
// every request in this subset addresses a document by.
func pathToURI(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path to uri: %w", ErrRelativePath)
	}

	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}

	return u.String(), nil
}

// uriToPath is the inverse of pathToURI, used to turn a server-returned
// Location.URI back into the plain path entities.Location carries.
func uriToPath(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("uri to path: %w", err)
	}

	if parsed.Scheme != "file" {
		return "", fmt.Errorf("uri to path: %w", ErrUnsupportedScheme)
	}

	return filepath.FromSlash(parsed.Path), nil
}
