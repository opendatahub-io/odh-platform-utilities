package deploy

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
//   - spec.template.spec.containers[].livenessProbe  (only when not set in desired)
//   - spec.template.spec.containers[].readinessProbe (only when not set in desired)
//   - spec.template.spec.containers[].startupProbe   (only when not set in desired)
//   - spec.replicas
func MergeDeployments(existing *unstructured.Unstructured, desired *unstructured.Unstructured) error {
	err := mergeContainerResources(existing, desired)
	if err != nil {
		return err
	}

	err = mergeContainerProbes(existing, desired)
	if err != nil {
		return err
	}

	return mergeReplicas(existing, desired)
}

func containersPath() []string {
	return []string{"spec", "template", "spec", "containers"}
}

func probeFields() []string {
	return []string{"livenessProbe", "readinessProbe", "startupProbe"}
}

func mergeContainerResources(existing, desired *unstructured.Unstructured) error {
	sourceContainers, err := extractContainers(existing.Object, containersPath())
	if err != nil {
		return err
	}

	targetContainers, err := extractContainers(desired.Object, containersPath())
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

// Note: probes absent from desired are preserved from live indefinitely —
// the same behaviour as container resources. Kubernetes has no empty-probe
// sentinel, so absence and explicit removal are indistinguishable here.
// A probe set in the live cluster cannot be removed via the rendered manifest.
func mergeContainerProbes(existing, desired *unstructured.Unstructured) error {
	sourceContainers, err := extractContainers(existing.Object, containersPath())
	if err != nil {
		return err
	}

	targetContainers, err := extractContainers(desired.Object, containersPath())
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

		probes := make(map[string]any, len(probeFields()))
		for _, field := range probeFields() {
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

		for _, field := range probeFields() {
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
	replicasPath := []string{"spec", "replicas"}

	containers, err := extractContainers(obj.Object, containersPath())
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
