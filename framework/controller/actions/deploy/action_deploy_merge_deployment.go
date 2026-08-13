package deploy

import (
	rootdeploy "github.com/opendatahub-io/odh-platform-utilities/pkg/deploy"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func MergeDeployments(source *unstructured.Unstructured, target *unstructured.Unstructured) error {
	return rootdeploy.MergeDeployments(source, target)
}
