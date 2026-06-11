package releases_test

import (
	"os"
	"path/filepath"
	"testing"

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

func TestFetchReleasesStatusAction(t *testing.T) {
	t.Helper()

	g := NewWithT(t)
	ctx := t.Context()

	tempDir := t.TempDir()

	tests := []struct {
		name             string
		metadataFilePath string
		metadataContent  string
		expectedReleases int
		expectedError    bool
		providedStatus   *common.ComponentReleaseStatus
	}{
		{
			name:             "should successfully render releases from valid YAML",
			metadataFilePath: filepath.Join(tempDir, "valid_file.yaml"),
			metadataContent:  metadataValidTwoReleases,
			expectedReleases: 2,
			expectedError:    false,
		},
		{
			name:             "should handle empty metadata file and return empty releases",
			metadataFilePath: filepath.Join(tempDir, "empty_file.yaml"),
			metadataContent:  "",
			expectedReleases: 0,
			expectedError:    false,
		},
		{
			name:             "should fail if YAML is invalid and return empty releases",
			metadataFilePath: filepath.Join(tempDir, "invalid_file.yaml"),
			metadataContent:  metadataUnknownField,
			expectedReleases: 0,
			expectedError:    false,
		},
		{
			name:             "should handle empty metadata file path gracefully",
			metadataFilePath: "",
			metadataContent:  "",
			expectedReleases: 0,
			expectedError:    false,
		},
		{
			name:             "should not re-render releases if cached",
			metadataFilePath: filepath.Join(tempDir, "cached_file.yaml"),
			metadataContent:  metadataOneRelease,
			expectedReleases: 1,
			expectedError:    false,
			providedStatus: &common.ComponentReleaseStatus{
				Releases: []common.ComponentRelease{
					{
						Name:    "Kubeflow Pipelines",
						Version: "0.0.0",
						RepoURL: "https://github.com/kubeflow/kfp-tekton",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.metadataContent != "" && tt.metadataFilePath != "" {
				err := os.MkdirAll(filepath.Dir(tt.metadataFilePath), 0755)
				if err != nil {
					t.Fatalf("failed to create directories: %v", err)
				}

				err = os.WriteFile(tt.metadataFilePath, []byte(tt.metadataContent), 0600)
				if err != nil {
					t.Fatalf("failed to write file: %v", err)
				}
			}

			instance := &fakeInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mock-instance",
				},
			}

			rr := types.ReconciliationRequest{
				Instance: instance,
			}

			opts := []releases.ActionOpts{
				releases.WithMetadataFilePath(func(_ *types.ReconciliationRequest) string {
					return tt.metadataFilePath
				}),
			}
			if tt.providedStatus != nil {
				opts = append(opts, releases.WithComponentReleaseStatus(*tt.providedStatus))
			}

			action := releases.NewAction(opts...)

			err := action(ctx, &rr)

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
