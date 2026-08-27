package deploy

import (
	"errors"

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
