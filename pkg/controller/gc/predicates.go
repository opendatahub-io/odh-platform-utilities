package gc

import (
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	odhAnnotations "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

// ObjectPredicateFn determines whether a specific object should be deleted.
// Returns true if the object is eligible for deletion.
type ObjectPredicateFn func(params RunParams, obj unstructured.Unstructured) (bool, error)

// TypePredicateFn determines whether a resource type (GVK) should be
// considered for GC. Returns true if the type should be scanned.
type TypePredicateFn func(params RunParams, gvk schema.GroupVersionKind) (bool, error)

// DefaultObjectPredicate determines if a resource is stale by checking the
// deploy/GC annotation protocol. A resource is eligible for deletion when:
//   - Any lifecycle annotation is missing (pre-annotation resource)
//   - The version, platform type, or instance UID changed
//   - The instance generation does not match the deploy-time generation
//
// Resources with no annotations at all are skipped (not managed by the deploy
// framework).
func DefaultObjectPredicate(params RunParams, obj unstructured.Unstructured) (bool, error) {
	if obj.GetAnnotations() == nil {
		return false, nil
	}

	pv := resources.GetAnnotation(&obj, odhAnnotations.PlatformVersion)
	pt := resources.GetAnnotation(&obj, odhAnnotations.PlatformType)
	ig := resources.GetAnnotation(&obj, odhAnnotations.InstanceGeneration)
	iu := resources.GetAnnotation(&obj, odhAnnotations.InstanceUID)

	if pv == "" || pt == "" || ig == "" || iu == "" {
		return true, nil
	}

	if pv != params.Version {
		return true, nil
	}

	if pt != params.PlatformType {
		return true, nil
	}

	if iu != string(params.Owner.GetUID()) {
		return true, nil
	}

	g, err := strconv.Atoi(ig)
	if err != nil {
		return false, fmt.Errorf("cannot determine generation: %w", err)
	}

	return params.Owner.GetGeneration() != int64(g), nil
}

// DefaultTypePredicate allows all resource types to be considered for GC.
func DefaultTypePredicate(_ RunParams, _ schema.GroupVersionKind) (bool, error) {
	return true, nil
}
