package releases_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/releases"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"

	. "github.com/onsi/gomega"
)

type fakeInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	status        api.Status
	releaseStatus common.ComponentReleaseStatus
}

func (f *fakeInstance) GetStatus() *api.Status {
	return &f.status
}

func (f *fakeInstance) GetConditions() []api.Condition {
	return f.status.Conditions
}

func (f *fakeInstance) SetConditions(c []api.Condition) {
	f.status.Conditions = c
}

func (f *fakeInstance) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &f.releaseStatus
}

func (f *fakeInstance) SetReleaseStatus(status common.ComponentReleaseStatus) {
	f.releaseStatus = status
}

func (f *fakeInstance) DeepCopyObject() runtime.Object {
	o := *f
	return &o
}

const metadataValidTwoReleases = `
releases:
  - name: Kubeflow Pipelines
    version: 2.2.0
    repoUrl: https://github.com/kubeflow/kfp-tekton
  - name: Another Component
    version: 1.3.1
    repoUrl: https://example.com/repo
`

const metadataUnknownField = `
releases:
  - name: Kubeflow Pipelines
    versionNumber: 2.2.0
    repoUrl: https://github.com/kubeflow/kfp-tekton
`

const metadataOneRelease = `
releases:
  - name: Kubeflow Pipelines
    version: 2.2.0
    repoUrl: https://github.com/kubeflow/kfp-tekton
`

// TestFetchReleasesStatusAction_CustomFS tests the action with an explicit in-memory
// filesystem and an explicit path, exercising YAML parsing and error handling.
func TestFetchReleasesStatusAction_CustomFS(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name             string
		mapFS            fstest.MapFS
		metadataFilePath string
		expectedReleases int
		expectedError    bool
		providedStatus   *common.ComponentReleaseStatus
	}{
		{
			name: "valid YAML returns all releases",
			mapFS: fstest.MapFS{
				"valid_file.yaml": &fstest.MapFile{Data: []byte(metadataValidTwoReleases)},
			},
			metadataFilePath: "valid_file.yaml",
			expectedReleases: 2,
		},
		{
			name: "empty file returns zero releases",
			mapFS: fstest.MapFS{
				"empty_file.yaml": &fstest.MapFile{Data: []byte("")},
			},
			metadataFilePath: "empty_file.yaml",
			expectedReleases: 0,
		},
		{
			name: "unknown YAML fields are skipped",
			mapFS: fstest.MapFS{
				"invalid_file.yaml": &fstest.MapFile{Data: []byte(metadataUnknownField)},
			},
			metadataFilePath: "invalid_file.yaml",
			expectedReleases: 0,
		},
		{
			name:             "missing file returns zero releases without error",
			mapFS:            fstest.MapFS{},
			metadataFilePath: "nonexistent_file.yaml",
			expectedReleases: 0,
		},
		{
			name: "cached status is used without re-reading the file",
			mapFS: fstest.MapFS{
				"cached_file.yaml": &fstest.MapFile{Data: []byte(metadataOneRelease)},
			},
			metadataFilePath: "cached_file.yaml",
			expectedReleases: 1,
			providedStatus: &common.ComponentReleaseStatus{
				Releases: []common.ComponentRelease{
					{Name: "Kubeflow Pipelines", Version: "0.0.0", RepoURL: "https://github.com/kubeflow/kfp-tekton"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			instance := &fakeInstance{ObjectMeta: metav1.ObjectMeta{Name: "mock-instance"}}
			rr := types.ReconciliationRequest{Instance: instance}

			opts := []releases.ActionOpts{
				releases.WithFS(tt.mapFS),
				releases.WithMetadataFilePath(func(_ *types.ReconciliationRequest) string {
					return tt.metadataFilePath
				}),
			}
			if tt.providedStatus != nil {
				opts = append(opts, releases.WithComponentReleaseStatus(*tt.providedStatus))
			}

			err := releases.NewAction(opts...)(ctx, &rr)

			if tt.expectedError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			finalReleases := instance.GetReleaseStatus()
			if tt.providedStatus != nil {
				g.Expect(finalReleases.Releases).To(Equal(tt.providedStatus.Releases))
			}
			g.Expect(finalReleases.Releases).To(HaveLen(tt.expectedReleases))
		})
	}
}

// TestFetchReleasesStatusAction_DefaultPath tests that the default metadataFilePathFn
// derives the path from ManifestsBasePath + lowercase(kind) + ComponentMetadataFilename.
func TestFetchReleasesStatusAction_DefaultPath(t *testing.T) {
	ctx := t.Context()

	const kind = "MyComponent"

	tests := []struct {
		name              string
		manifestsBasePath string
		fileAtPath        string
		expectedReleases  int
	}{
		{
			name:              "absolute ManifestsBasePath is stripped of leading slash for fs.FS",
			manifestsBasePath: "/manifests",
			fileAtPath:        "manifests/mycomponent/component_metadata.yaml",
			expectedReleases:  2,
		},
		{
			name:              "relative ManifestsBasePath is used as-is",
			manifestsBasePath: "manifests",
			fileAtPath:        "manifests/mycomponent/component_metadata.yaml",
			expectedReleases:  2,
		},
		{
			name:              "empty ManifestsBasePath uses only kind and filename",
			manifestsBasePath: "",
			fileAtPath:        "mycomponent/component_metadata.yaml",
			expectedReleases:  2,
		},
		{
			name:              "missing file returns zero releases without error",
			manifestsBasePath: "/manifests",
			fileAtPath:        "",
			expectedReleases:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			mapFS := fstest.MapFS{}
			if tt.fileAtPath != "" {
				mapFS[tt.fileAtPath] = &fstest.MapFile{Data: []byte(metadataValidTwoReleases)}
			}

			instance := &fakeInstance{
				TypeMeta:   metav1.TypeMeta{Kind: kind},
				ObjectMeta: metav1.ObjectMeta{Name: "mock-instance"},
			}
			rr := types.ReconciliationRequest{
				Instance:          instance,
				ManifestsBasePath: tt.manifestsBasePath,
			}

			err := releases.NewAction(releases.WithFS(mapFS))(ctx, &rr)

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(instance.GetReleaseStatus().Releases).To(HaveLen(tt.expectedReleases))
		})
	}
}

// TestFetchReleasesStatusAction_DefaultFS tests that the default os.DirFS("/") is used
// when no WithFS option is provided, reading from the real filesystem.
func TestFetchReleasesStatusAction_DefaultFS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	const kind = "MyComponent"

	tempDir := t.TempDir()
	metadataDir := filepath.Join(tempDir, "mycomponent")

	err := os.MkdirAll(metadataDir, 0o750)
	g.Expect(err).NotTo(HaveOccurred())

	metadataFile := filepath.Join(metadataDir, releases.ComponentMetadataFilename)
	err = os.WriteFile(metadataFile, []byte(metadataValidTwoReleases), 0o600)
	g.Expect(err).NotTo(HaveOccurred())

	instance := &fakeInstance{
		TypeMeta:   metav1.TypeMeta{Kind: kind},
		ObjectMeta: metav1.ObjectMeta{Name: "mock-instance"},
	}
	rr := types.ReconciliationRequest{
		Instance:          instance,
		ManifestsBasePath: tempDir,
	}

	err = releases.NewAction()(ctx, &rr)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(instance.GetReleaseStatus().Releases).To(HaveLen(2))
}
