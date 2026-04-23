package deploy

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// MergeFn defines a strategy for merging fields from the existing (live)
// resource into the desired (rendered) resource before apply. This preserves
// user customisations that SSA would otherwise overwrite.
type MergeFn func(existing, desired *unstructured.Unstructured) error

var (
	// ErrFieldNotSlice is returned when a containers field is not a slice.
	ErrFieldNotSlice = errors.New("field is not a slice")
	// ErrFieldNotMap is returned when a container element is not a map.
	ErrFieldNotMap = errors.New("field is not a map")
)

// MergeDeployments preserves user-set container resources and replicas when
// SSA-applying updated Deployment manifests. Without this merge, SSA would
// reclaim ownership of these fields and reset user customisations.
//
// Fields merged from existing -> desired:
//   - spec.template.spec.containers[].resources  (matched by container name)
//   - spec.replicas
func MergeDeployments(existing *unstructured.Unstructured, desired *unstructured.Unstructured) error {
	err := mergeContainerResources(existing, desired)
	if err != nil {
		return err
	}

	return mergeReplicas(existing, desired)
}

func mergeContainerResources(existing, desired *unstructured.Unstructured) error {
	containersPath := []string{"spec", "template", "spec", "containers"}

	sourceContainers, err := extractContainers(existing.Object, containersPath)
	if err != nil {
		return err
	}

	targetContainers, err := extractContainers(desired.Object, containersPath)
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
			m["resources"] = nr
		}
	}
}

func mergeReplicas(existing, desired *unstructured.Unstructured) error {
	replicasPath := []string{"spec", "replicas"}

	sourceReplica, ok, err := unstructured.NestedFieldNoCopy(existing.Object, replicasPath...)
	if err != nil {
		return err
	}

	if !ok {
		unstructured.RemoveNestedField(desired.Object, replicasPath...)
		return nil
	}

	return unstructured.SetNestedField(desired.Object, sourceReplica, replicasPath...)
}

// MergeObservabilityResources preserves user-set spec.resources from the
// existing resource. This is intended for observability stack types
// (MonitoringStack, TempoStack, OpenTelemetryCollector, etc.) where users may
// tune resource requests/limits.
func MergeObservabilityResources(existing *unstructured.Unstructured, desired *unstructured.Unstructured) error {
	resourcesPath := []string{"spec", "resources"}

	sourceResources, ok, err := unstructured.NestedFieldNoCopy(existing.Object, resourcesPath...)
	if err != nil {
		return err
	}

	if ok && sourceResources != nil {
		return unstructured.SetNestedField(desired.Object, sourceResources, resourcesPath...)
	}

	return nil
}

// RemoveDeploymentResources strips container resources and replicas from a
// Deployment manifest. This is used in patch mode to avoid overwriting user
// customisations.
func RemoveDeploymentResources(obj *unstructured.Unstructured) error {
	containersPath := []string{"spec", "template", "spec", "containers"}
	replicasPath := []string{"spec", "replicas"}

	containers, err := extractContainers(obj.Object, containersPath)
	if err != nil {
		return fmt.Errorf("extract containers: %w", err)
	}

	for i := range containers {
		m, ok := containers[i].(map[string]any)
		if !ok {
			return ErrFieldNotMap
		}

		delete(m, "resources")
	}

	unstructured.RemoveNestedField(obj.Object, replicasPath...)

	return nil
}
