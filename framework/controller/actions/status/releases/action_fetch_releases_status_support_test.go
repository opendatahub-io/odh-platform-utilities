package releases_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/releases"

	. "github.com/onsi/gomega"
)

func TestNormalizeComponentReleases(t *testing.T) {
	tests := []struct {
		name     string
		input    []api.ComponentRelease
		expected []api.ComponentRelease
	}{
		{
			name:     "nil input returns empty",
			input:    nil,
			expected: []api.ComponentRelease{},
		},
		{
			name:     "empty input returns empty",
			input:    []api.ComponentRelease{},
			expected: []api.ComponentRelease{},
		},
		{
			name: "empty version is filtered out",
			input: []api.ComponentRelease{
				{Name: "A", Version: ""},
				{Name: "B", Version: "1.0.0"},
			},
			expected: []api.ComponentRelease{
				{Name: "B", Version: "1.0.0"},
			},
		},
		{
			name: "whitespace-only version is filtered out",
			input: []api.ComponentRelease{
				{Name: "A", Version: "  "},
				{Name: "B", Version: "1.0.0"},
			},
			expected: []api.ComponentRelease{
				{Name: "B", Version: "1.0.0"},
			},
		},
		{
			name: "version whitespace is trimmed",
			input: []api.ComponentRelease{
				{Name: "A", Version: "  2.0.0  "},
			},
			expected: []api.ComponentRelease{
				{Name: "A", Version: "2.0.0"},
			},
		},
		{
			name: "result is sorted by name",
			input: []api.ComponentRelease{
				{Name: "Zebra", Version: "1.0.0"},
				{Name: "Alpha", Version: "2.0.0"},
				{Name: "Mango", Version: "3.0.0"},
			},
			expected: []api.ComponentRelease{
				{Name: "Alpha", Version: "2.0.0"},
				{Name: "Mango", Version: "3.0.0"},
				{Name: "Zebra", Version: "1.0.0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(releases.NormalizeComponentReleases(tt.input)).To(Equal(tt.expected))
		})
	}
}

func TestParseComponentReleases(t *testing.T) {
	tests := []struct {
		name          string
		input         []byte
		expectedLen   int
		expectedError bool
	}{
		{
			name:        "valid YAML returns releases",
			input:       []byte(metadataValidTwoReleases),
			expectedLen: 2,
		},
		{
			name:        "empty input returns no releases",
			input:       []byte(""),
			expectedLen: 0,
		},
		{
			name:        "unknown field skips version, release is filtered",
			input:       []byte(metadataUnknownField),
			expectedLen: 0,
		},
		{
			name:          "invalid YAML returns error",
			input:         []byte("releases: [\nnot valid yaml"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			result, err := releases.ParseComponentReleases(tt.input)

			if tt.expectedError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result).To(HaveLen(tt.expectedLen))
			}
		})
	}
}

func TestReadComponentReleases(t *testing.T) {
	tests := []struct {
		name          string
		fsys          fstest.MapFS
		path          string
		expectedLen   int
		expectedError error
	}{
		{
			name: "existing file returns releases",
			fsys: fstest.MapFS{
				"meta.yaml": &fstest.MapFile{Data: []byte(metadataValidTwoReleases)},
			},
			path:        "meta.yaml",
			expectedLen: 2,
		},
		{
			name:          "missing file returns ErrNotExist",
			fsys:          fstest.MapFS{},
			path:          "nonexistent.yaml",
			expectedError: fs.ErrNotExist,
		},
		{
			name: "empty file returns no releases",
			fsys: fstest.MapFS{
				"empty.yaml": &fstest.MapFile{Data: []byte("")},
			},
			path:        "empty.yaml",
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			result, err := releases.ReadComponentReleases(tt.fsys, tt.path)

			if tt.expectedError != nil {
				g.Expect(err).To(MatchError(tt.expectedError))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(result).To(HaveLen(tt.expectedLen))
			}
		})
	}
}
