// framework/resources/source.go
package resources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
)

// Source loads raw bytes from a location.
type Source interface {
	Load(ctx context.Context) ([]byte, error)
}

// URLSource loads content from an HTTPS URL.
type URLSource struct {
	URL    *url.URL
	Client *http.Client
}

// NewURLSource parses raw as an HTTPS URL and returns a URLSource using http.DefaultClient.
// Returns an error if raw is not a valid URL, does not use https, has an empty host,
// or contains userinfo (credentials in URLs leak into logs and error messages).
func NewURLSource(raw string) (URLSource, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return URLSource{}, fmt.Errorf("parse URL: %w", err)
	}
	if u.Scheme != "https" {
		return URLSource{}, fmt.Errorf("URL scheme must be https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return URLSource{}, fmt.Errorf("URL must have a non-empty host: %q", raw)
	}
	if u.User != nil {
		return URLSource{}, errors.New("URL must not contain credentials; use a separate auth mechanism")
	}
	return URLSource{URL: u, Client: http.DefaultClient}, nil
}

// manifestReadLimit caps the response body to prevent OOM on unexpectedly large responses.
const manifestReadLimit = 50 << 20 // 50 MiB

// Load fetches the content from the URL.
func (s URLSource) Load(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", s.URL.Redacted(), err)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", s.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", s.URL.Redacted(), resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, manifestReadLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read body %s: %w", s.URL.Redacted(), err)
	}
	if int64(len(body)) > manifestReadLimit {
		return nil, fmt.Errorf("fetch %s: response body exceeds 50 MiB limit", s.URL.Redacted())
	}
	return body, nil
}

// FileSource loads content from a filesystem path.
// Uses fs.FS for testability — pass os.DirFS("/") for real filesystem access,
// or an embed.FS / fstest.MapFS for testing.
type FileSource struct {
	FS   fs.FS
	Path string
}

// NewFileSource creates a FileSource that reads from the real filesystem.
// path must be absolute (start with "/").
func NewFileSource(path string) (FileSource, error) {
	if len(path) == 0 || path[0] != '/' {
		return FileSource{}, fmt.Errorf("path must be absolute, got %q", path)
	}
	stripped := path[1:]
	if !fs.ValidPath(stripped) {
		return FileSource{}, fmt.Errorf("invalid path: %q", path)
	}
	return FileSource{FS: os.DirFS("/"), Path: stripped}, nil
}

// Load reads the file content.
func (s FileSource) Load(_ context.Context) ([]byte, error) {
	if !fs.ValidPath(s.Path) {
		return nil, fmt.Errorf("invalid path: %q", s.Path)
	}
	data, err := fs.ReadFile(s.FS, s.Path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", s.Path, err)
	}
	return data, nil
}
