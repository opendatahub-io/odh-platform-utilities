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
	deploymentProbeFields    = []string{"livenessProbe", "readinessProbe", "startupProbe"}
)

// MergeDeployments merges fields from the existing (live) Deployment into the
// desired (rendered) Deployment before SSA apply, preserving user-tuned values
// that SSA would otherwise overwrite. Two merge policies apply:
//
//   - resources and replicas: live value always wins. An absent resources map
//     on live deletes desired resources.
//
//   - probes (liveness, readiness, startup): fill-if-absent. A probe present
//     in desired is never overwritten. A probe absent from desired is copied
//     from live if one exists there. Because Kubernetes has no empty-probe
//     sentinel, a probe set on a live Deployment cannot be removed by omitting
//     it from the rendered manifest.
//
// Fields merged from existing -> desired:
//   - spec.replicas
//   - spec.template.spec.containers[].resources  (matched by container name; live always wins)
//   - spec.template.spec.containers[].livenessProbe  (only when not set in desired)
//   - spec.template.spec.containers[].readinessProbe (only when not set in desired)
//   - spec.template.spec.containers[].startupProbe   (only when not set in desired)
func MergeDeployments(source *unstructured.Unstructured, target *unstructured.Unstructured) error {
	if err := mergeContainerResources(source, target); err != nil {
		return err
	}

	if err := mergeContainerProbes(source, target); err != nil {
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

// Probes use fill-if-absent semantics: a probe present in target is never
// overwritten; a probe absent from target is copied from source if one exists.
// This differs from resource merging, where source always wins and an empty
// source map deletes target resources.
func mergeContainerProbes(source, target *unstructured.Unstructured) error {
	sourceContainers, err := extractContainers(source.Object, deploymentContainersPath)
	if err != nil {
		return err
	}

	targetContainers, err := extractContainers(target.Object, deploymentContainersPath)
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
