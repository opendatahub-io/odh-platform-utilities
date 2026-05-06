package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrNotDeployment is returned when RevertDeploymentDrift receives a non-Deployment object.
	ErrNotDeployment = errors.New("expected Deployment GVK")
	// ErrNilArgument is returned when RevertDeploymentDrift receives a nil obj, old, or client.
	ErrNilArgument = errors.New("nil argument")
)

//nolint:gochecknoglobals
var driftDeploymentGVK = schema.GroupVersionKind{
	Group:   appsv1.SchemeGroupVersion.Group,
	Version: appsv1.SchemeGroupVersion.Version,
	Kind:    "Deployment",
}

// RevertDeploymentDrift performs a Strategic Merge Patch to clear user
// modifications to managed Deployment fields when drift is detected.
//
// SSA can only manage fields that are present in the manifest. When the
// manifest intentionally omits a field (e.g. empty resources/replicas), SSA
// cannot remove user-owned values. Strategic Merge Patch can explicitly set
// fields to nil, clearing user modifications. After this patch, SSA with
// ForceOwnership reclaims ownership of the manifest fields.
//
// Managed fields (only when absent from manifest):
//   - Container resources (requests/limits): cleared if manifest omits them
//   - Replicas: set to nil (Kubernetes defaults to 1) if manifest omits it
func RevertDeploymentDrift(
	ctx context.Context,
	cli client.Client,
	obj *unstructured.Unstructured,
	old *unstructured.Unstructured,
) error {
	if cli == nil {
		return fmt.Errorf("%w: client is nil", ErrNilArgument)
	}

	if obj == nil {
		return fmt.Errorf("%w: obj is nil", ErrNilArgument)
	}

	if old == nil {
		return fmt.Errorf("%w: old is nil", ErrNilArgument)
	}

	if obj.GroupVersionKind() != driftDeploymentGVK {
		return fmt.Errorf("%w: got %s", ErrNotDeployment, obj.GroupVersionKind())
	}

	if old.GroupVersionKind() != driftDeploymentGVK {
		return fmt.Errorf("%w: got %s", ErrNotDeployment, old.GroupVersionKind())
	}

	containerPatches, err := computeContainerPatches(obj, old)
	if err != nil {
		return err
	}

	replicaPatchNeeded, err := needsReplicaPatch(obj, old)
	if err != nil {
		return err
	}

	if !replicaPatchNeeded && len(containerPatches) == 0 {
		return nil
	}

	return applyDriftPatch(ctx, cli, obj, old, containerPatches, replicaPatchNeeded)
}

func computeContainerPatches(obj, old *unstructured.Unstructured) ([]map[string]any, error) {
	containersPath := []string{"spec", "template", "spec", "containers"}

	objContainers, objFound, err := unstructured.NestedSlice(obj.Object, containersPath...)
	if err != nil {
		return nil, fmt.Errorf("failed to get containers from manifest: %w", err)
	}

	oldContainers, oldFound, err := unstructured.NestedSlice(old.Object, containersPath...)
	if err != nil {
		return nil, fmt.Errorf("failed to get containers from deployed object: %w", err)
	}

	if !objFound || !oldFound {
		return nil, nil
	}

	var patches []map[string]any

	for _, objCont := range objContainers {
		patch := matchContainerPatch(objCont, oldContainers)
		if patch != nil {
			patches = append(patches, patch)
		}
	}

	return patches, nil
}

func matchContainerPatch(objCont any, oldContainers []any) map[string]any {
	objMap, ok := objCont.(map[string]any)
	if !ok {
		return nil
	}

	objName, ok := objMap["name"].(string)
	if !ok {
		return nil
	}

	for _, oldCont := range oldContainers {
		oldMap, ok := oldCont.(map[string]any)
		if !ok {
			continue
		}

		oldName, ok := oldMap["name"].(string)
		if !ok || oldName != objName {
			continue
		}

		return buildContainerResourcePatch(objMap, oldMap, objName)
	}

	return nil
}

func buildContainerResourcePatch(objMap, oldMap map[string]any, name string) map[string]any {
	objResources, objHasResources := objMap["resources"]
	if objHasResources {
		objHasResources = !isEmptyResourceMap(objResources)
	}

	_, oldHasResources := oldMap["resources"]

	if oldHasResources && !objHasResources {
		return map[string]any{
			"name":      name,
			"resources": nil,
		}
	}

	if objHasResources && oldHasResources {
		oldResources := oldMap["resources"]
		if !equality.Semantic.DeepEqual(objResources, oldResources) {
			return buildResourcesPatch(name, objResources, oldResources)
		}
	}

	return nil
}

func needsReplicaPatch(obj, old *unstructured.Unstructured) (bool, error) {
	replicasPath := []string{"spec", "replicas"}

	_, objHasReplicas, err := unstructured.NestedInt64(obj.Object, replicasPath...)
	if err != nil {
		return false, fmt.Errorf("failed to get replicas from manifest: %w", err)
	}

	_, oldHasReplicas, err := unstructured.NestedInt64(old.Object, replicasPath...)
	if err != nil {
		return false, fmt.Errorf("failed to get replicas from deployed object: %w", err)
	}

	return oldHasReplicas && !objHasReplicas, nil
}

func applyDriftPatch(
	ctx context.Context,
	cli client.Client,
	obj, old *unstructured.Unstructured,
	containerPatches []map[string]any,
	replicaPatchNeeded bool,
) error {
	spec := map[string]any{}
	patchData := map[string]any{"spec": spec}

	if len(containerPatches) > 0 {
		spec["template"] = map[string]any{
			"spec": map[string]any{
				"containers": containerPatches,
			},
		}
	}

	if replicaPatchNeeded {
		spec["replicas"] = nil
	}

	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return fmt.Errorf("failed to marshal patch data for Deployment %s/%s: %w",
			obj.GetNamespace(), obj.GetName(), err)
	}

	err = cli.Patch(ctx, old, client.RawPatch(types.StrategicMergePatchType, patchBytes))
	if err != nil {
		return fmt.Errorf("failed to patch managed Deployment %s/%s: %w",
			obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}

func isEmptyResourceMap(v any) bool {
	m, ok := v.(map[string]any)
	return ok && len(m) == 0
}

func buildResourcesPatch(name string, manifestResources, deployedResources any) map[string]any {
	manifestMap, ok := manifestResources.(map[string]any)
	if !ok {
		return nil
	}

	deployedMap, ok := deployedResources.(map[string]any)
	if !ok {
		return nil
	}

	patchResources := make(map[string]any)

	for _, field := range []string{"requests", "limits"} {
		manifest, manifestFound := manifestMap[field].(map[string]any)
		deployed, deployedFound := deployedMap[field].(map[string]any)

		if !manifestFound && !deployedFound {
			continue
		}

		merged := make(map[string]any, len(manifest)+len(deployed))
		maps.Copy(merged, manifest)

		for key := range deployed {
			if _, exists := manifest[key]; !exists {
				merged[key] = nil
			}
		}

		if len(merged) > 0 {
			patchResources[field] = merged
		}
	}

	return map[string]any{"name": name, "resources": patchResources}
}
