package releases

import (
	"cmp"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
)

// NormalizeComponentReleases filters out releases with empty versions, trims whitespace
// from versions, and returns the result sorted by Name.
func NormalizeComponentReleases(releases []api.ComponentRelease) []api.ComponentRelease {
	result := make([]api.ComponentRelease, 0, len(releases))
	for _, r := range releases {
		normalized := api.ComponentRelease{
			Name:    r.Name,
			Version: strings.TrimSpace(r.Version),
			RepoURL: r.RepoURL,
		}
		if normalized.Version == "" {
			continue
		}
		result = append(result, normalized)
	}

	slices.SortFunc(result, func(a, b api.ComponentRelease) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return result
}

// ParseComponentReleases unmarshals raw YAML bytes into a list of normalized releases.
func ParseComponentReleases(data []byte) ([]api.ComponentRelease, error) {
	var meta api.ComponentReleaseStatus
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("error unmarshaling YAML: %w", err)
	}

	return NormalizeComponentReleases(meta.Releases), nil
}

// ReadComponentReleases reads the file at path from fsys and returns normalized releases.
// Errors are wrapped with path context but preserve fs.ErrNotExist for callers to inspect.
func ReadComponentReleases(fsys fs.FS, path string) ([]api.ComponentRelease, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("reading release status from %q: %w", path, err)
	}

	return ParseComponentReleases(data)
}
