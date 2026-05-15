package testutil_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/api/common/testutil"
)

func TestValidatePlatformObject_ValidImplementation(t *testing.T) {
	t.Parallel()

	obj := &fakeModule{
		ObjectMeta: metav1.ObjectMeta{Name: "test-module"},
	}

	testutil.ValidatePlatformObject(t, obj)
}

// --- fakeModule: correct PlatformObject implementation -------------------

type fakeModuleStatus struct {
	common.ComponentReleaseStatus `json:",inline"`
	common.Status                 `json:",inline"`
}

type fakeModule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status fakeModuleStatus `json:"status,omitempty"`
}

func (f *fakeModule) GetStatus() *common.Status {
	return &f.Status.Status
}

func (f *fakeModule) GetConditions() []common.Condition {
	return f.Status.Conditions
}

func (f *fakeModule) SetConditions(conditions []common.Condition) {
	f.Status.Conditions = conditions
}

func (f *fakeModule) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &f.Status.ComponentReleaseStatus
}

func (f *fakeModule) SetReleaseStatus(
	status common.ComponentReleaseStatus,
) {
	f.Status.ComponentReleaseStatus = status
}

func (f *fakeModule) DeepCopyObject() runtime.Object {
	return f
}
