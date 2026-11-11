package deploy

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	// ErrFieldNotSlice is returned when a containers field is not a slice.
	ErrFieldNotSlice = errors.New("field is not a slice")
	// ErrFieldNotMap is returned when a container element is not a map.
	ErrFieldNotMap = errors.New("field is not a map")

	deploymentContainersPath = []string{"spec", "template", "spec", "containers"}
)

// MergeDeployments merges fields from the existing (live) Deployment into the
// desired (rendered) Deployment before SSA apply, preserving user-tuned values
// that SSA would otherwise overwrite. The live value always wins; an absent
// resources map on live deletes desired resources.
//
// Fields merged from existing -> desired:
//   - spec.replicas
//   - spec.template.spec.containers[].resources  (matched by container name; live always wins)
func MergeDeployments(source *unstructured.Unstructured, target *unstructured.Unstructured) error {
	if err := mergeContainerResources(source, target); err != nil {
		return err
	}

	if err := mergeReplicas(source, target); err != nil {
		return err
	}

	return nil
}

func mergeContainerResources(source, target *unstructured.Unstructured) error {
	sourceContainers, err := extractContainers(source.Object, deploymentContainersPath)
	if err != nil {
		return err
	}

	targetContainers, err := extractContainers(target.Object, deploymentContainersPath)
	if err != nil {
		return err
	}

	resourcesByName := buildResourceMap(sourceContainers)
	applyResourceMap(targetContainers, resourcesByName)

	return nil
}

func extractContainers(obj map[string]any, path []string) ([]any, error) {
	raw, ok, err := unstructured.NestedFieldNoCopy(obj, path...)
	if err != nil {
		return nil, err
	}

	if !ok || raw == nil {
		return nil, nil
	}

	containers, ok := raw.([]any)
	if !ok {
		return nil, ErrFieldNotSlice
	}

	return containers, nil
}

func buildResourceMap(containers []any) map[string]any {
	result := make(map[string]any)

	for i := range containers {
		m, ok := containers[i].(map[string]any)
		if !ok {
			continue
		}

		name, ok := m["name"].(string)
		if !ok {
			continue
		}

		r, ok := m["resources"]
		if !ok {
			r = make(map[string]any)
		}

		result[name] = r
	}

	return result
}

func applyResourceMap(containers []any, resourcesByName map[string]any) {
	for i := range containers {
		m, ok := containers[i].(map[string]any)
		if !ok {
			continue
		}

		name, ok := m["name"].(string)
		if !ok {
			continue
		}

		nr, ok := resourcesByName[name]
		if !ok {
			continue
		}

		nrMap, _ := nr.(map[string]any)
		if len(nrMap) == 0 {
			delete(m, "resources")
		} else {
			m["resources"] = runtime.DeepCopyJSONValue(nr)
		}
	}
}

func mergeReplicas(source, target *unstructured.Unstructured) error {
	replicasPath := []string{"spec", "replicas"}

	sourceReplica, ok, err := unstructured.NestedFieldNoCopy(source.Object, replicasPath...)
	if err != nil {
		return err
	}

	if !ok {
		unstructured.RemoveNestedField(target.Object, replicasPath...)
		return nil
	}

	return unstructured.SetNestedField(target.Object, sourceReplica, replicasPath...)
}

// preserveDeploymentUserFields fills the given fragment with the live
// values of the Deployment user-tuned fields (spec.replicas and the
// container resources), so an owner applying the fragment re-claims the
// fields instead of deleting them: it may be their sole owner, because the
// current owner's write omits them (see patchWriteOmitsUserFields).
//
// Skeletons are created only for the containers the desired state still
// carries AND that exist live: a fragment naming a container the desired
// state dropped would retain its stale entry, and a fragment naming one
// the desired state adds would create a name-only entry (MergeDeployments
// fills resources only from a live container with the same name), which
// the API server rejects — the release would fail and the release-pending
// owner would wedge. MergeDeployments fills each skeleton with the live
// resources of the container with the same name, and the live replica
// count into spec.replicas.
//
// The live container names are read here rather than through
// deploymentContainerNames (which swallows read errors into a nil map)
// because a failed read must fail loudly: a silently empty intersection
// would degrade the fragment to identity-only and delete the user-tuned
// values it exists to preserve.
func preserveDeploymentUserFields(desired, live, fragment *unstructured.Unstructured) error {
	containers, _, err := unstructured.NestedSlice(desired.Object, deploymentContainersPath...)
	if err != nil {
		return fmt.Errorf("failed to read the containers of %s/%s for the release body: %w",
			desired.GetNamespace(), desired.GetName(), err)
	}

	liveContainers, _, err := unstructured.NestedSlice(live.Object, deploymentContainersPath...)
	if err != nil {
		return fmt.Errorf("failed to read the live containers of %s/%s for the release body: %w",
			desired.GetNamespace(), desired.GetName(), err)
	}

	liveNames := make(map[string]struct{}, len(liveContainers))
	for _, raw := range liveContainers {
		if container, ok := raw.(map[string]any); ok {
			if name, ok := container["name"].(string); ok {
				liveNames[name] = struct{}{}
			}
		}
	}

	skeletons := make([]any, 0, len(containers))
	for _, raw := range containers {
		container, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, ok := container["name"].(string)
		if !ok {
			continue
		}

		// A container the desired state adds has no live counterpart:
		// the previous owner never claimed it, so there is nothing to
		// preserve and nothing to release.
		if _, exists := liveNames[name]; !exists {
			continue
		}

		skeletons = append(skeletons, map[string]any{"name": name})
	}

	if len(skeletons) > 0 {
		if err := unstructured.SetNestedSlice(fragment.Object, skeletons, deploymentContainersPath...); err != nil {
			return fmt.Errorf("failed to set the containers of the release body for %s/%s: %w",
				desired.GetNamespace(), desired.GetName(), err)
		}
	}

	if err := MergeDeployments(live, fragment); err != nil {
		return fmt.Errorf("failed to merge the live Deployment values into the release body for %s/%s: %w",
			desired.GetNamespace(), desired.GetName(), err)
	}

	return nil
}
