package resources_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
)

func TestNewURLSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "valid https", raw: "https://example.com/manifest.yaml"},
		{name: "http rejected", raw: "http://example.com/manifest.yaml", wantErr: "https"},
		{name: "empty host", raw: "https:///manifest.yaml", wantErr: "host"},
		{name: "credentials rejected", raw: "https://user:pass@example.com/manifest.yaml", wantErr: "credentials"}, //nolint:gosec
		{name: "invalid url", raw: "://bad", wantErr: "parse URL"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src, err := resources.NewURLSource(tc.raw)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, src.Client)
		})
	}
}

func TestURLSource_Load(t *testing.T) {
	t.Parallel()

	t.Run("200 ok", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("content"))
		}))
		t.Cleanup(srv.Close)

		src, err := resources.NewURLSource(srv.URL)
		require.NoError(t, err)
		src.Client = srv.Client()

		data, err := src.Load(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []byte("content"), data)
	})

	t.Run("non-200 response", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)

		src, err := resources.NewURLSource(srv.URL)
		require.NoError(t, err)
		src.Client = srv.Client()

		_, err = src.Load(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("body exceeds limit", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Write 50 MiB + 1 byte to exceed the limit.
			chunk := strings.Repeat("x", 1024)
			for range (50 * 1024) + 1 {
				_, _ = w.Write([]byte(chunk))
			}
		}))
		t.Cleanup(srv.Close)

		src, err := resources.NewURLSource(srv.URL)
		require.NoError(t, err)
		src.Client = srv.Client()

		_, err = src.Load(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "50 MiB")
	})
}

func TestNewFileSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "valid absolute path", path: "/etc/hosts"},
		{name: "relative path rejected", path: "etc/hosts", wantErr: "absolute"},
		{name: "empty path rejected", path: "", wantErr: "absolute"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := resources.NewFileSource(tc.path)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestFileSource_Load(t *testing.T) {
	t.Parallel()

	t.Run("reads existing file", func(t *testing.T) {
		t.Parallel()
		fs := fstest.MapFS{"manifest.yaml": &fstest.MapFile{Data: []byte("data")}}
		src := resources.FileSource{FS: fs, Path: "manifest.yaml"}

		data, err := src.Load(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []byte("data"), data)
	})

	t.Run("missing file returns error", func(t *testing.T) {
		t.Parallel()
		fs := fstest.MapFS{}
		src := resources.FileSource{FS: fs, Path: "missing.yaml"}

		_, err := src.Load(context.Background())
		require.Error(t, err)
	})
}
