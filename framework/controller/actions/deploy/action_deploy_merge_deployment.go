package deploy

import (
	"errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	ErrFieldNotSlice = errors.New("field is not a slice")
	ErrFieldNotMap   = errors.New("field is not a map")

	deploymentContainersPath = []string{"spec", "template", "spec", "containers"}
	deploymentProbeFields    = []string{"livenessProbe", "readinessProbe", "startupProbe"}
)

// MergeDeployments preserves user-tuned fields from the live Deployment while
// still allowing rendered manifests to fill in missing probes.
func MergeDeployments(existing *unstructured.Unstructured, desired *unstructured.Unstructured) error {
	if err := mergeContainerResources(existing, desired); err != nil {
		return err
	}

	if err := mergeContainerProbes(existing, desired); err != nil {
		return err
	}

	if err := mergeReplicas(existing, desired); err != nil {
		return err
	}

	return nil
}

func mergeContainerResources(existing, desired *unstructured.Unstructured) error {
	sourceContainers, err := extractContainers(existing.Object, deploymentContainersPath)
	if err != nil {
		return err
	}

	targetContainers, err := extractContainers(desired.Object, deploymentContainersPath)
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

func mergeContainerProbes(existing, desired *unstructured.Unstructured) error {
	sourceContainers, err := extractContainers(existing.Object, deploymentContainersPath)
	if err != nil {
		return err
	}

	targetContainers, err := extractContainers(desired.Object, deploymentContainersPath)
	if err != nil {
		return err
	}

	probesByName := buildProbeMap(sourceContainers)
	applyProbeMap(targetContainers, probesByName)

	return nil
}

func buildProbeMap(containers []any) map[string]map[string]any {
	result := make(map[string]map[string]any, len(containers))

	for i := range containers {
		m, ok := containers[i].(map[string]any)
		if !ok {
			continue
		}

		name, ok := m["name"].(string)
		if !ok {
			continue
		}

		probes := make(map[string]any, len(deploymentProbeFields))
		for _, field := range deploymentProbeFields {
			if v, exists := m[field]; exists {
				probes[field] = v
			}
		}

		result[name] = probes
	}

	return result
}

func applyProbeMap(containers []any, probesByName map[string]map[string]any) {
	for i := range containers {
		m, ok := containers[i].(map[string]any)
		if !ok {
			continue
		}

		name, ok := m["name"].(string)
		if !ok {
			continue
		}

		liveProbes, ok := probesByName[name]
		if !ok {
			continue
		}

		for _, field := range deploymentProbeFields {
			if _, inDesired := m[field]; inDesired {
				continue
			}

			if probe, inLive := liveProbes[field]; inLive {
				m[field] = runtime.DeepCopyJSONValue(probe)
			}
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
