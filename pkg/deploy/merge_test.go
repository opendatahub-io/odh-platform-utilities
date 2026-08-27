package deploy_test

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/deploy"

	. "github.com/onsi/gomega"
)

func TestMergeDeploymentsOverrideReplicas(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	source := deploymentFixture(g, ptr.To[int32](1), "3", "3Gi")
	target := deploymentFixture(g, ptr.To[int32](3), "1", "1Gi")

	err := deploy.MergeDeployments(source, target)
	g.Expect(err).ShouldNot(HaveOccurred())

	replicas, found, err := unstructured.NestedInt64(target.Object, "spec", "replicas")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(found).Should(BeTrue())
	g.Expect(replicas).Should(Equal(int64(1)))
}

func TestMergeDeploymentsOverrideResources(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	source := deploymentFixture(g, ptr.To[int32](1), "3", "3Gi")
	target := deploymentFixture(g, ptr.To[int32](3), "1", "1Gi")

	err := deploy.MergeDeployments(source, target)
	g.Expect(err).ShouldNot(HaveOccurred())

	containers, _, err := unstructured.NestedSlice(target.Object, "spec", "template", "spec", "containers")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(containers).Should(HaveLen(1))

	cm, ok := containers[0].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	res, ok := cm["resources"].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	reqs, ok := res["requests"].(map[string]any)
	g.Expect(ok).Should(BeTrue())
	g.Expect(reqs["cpu"]).Should(Equal("3"))
	g.Expect(reqs["memory"]).Should(Equal("3Gi"))
}

func TestMergeDeploymentsRemove(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	source := toUnstructured(g, &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "test"}},
				},
			},
		},
	})

	target := toUnstructured(g, &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](3),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
						},
					}},
				},
			},
		},
	})

	err := deploy.MergeDeployments(source, target)
	g.Expect(err).ShouldNot(HaveOccurred())

	_, found, err := unstructured.NestedInt64(target.Object, "spec", "replicas")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(found).Should(BeFalse())

	containers, _, err := unstructured.NestedSlice(target.Object, "spec", "template", "spec", "containers")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(containers).Should(HaveLen(1))
	cm, ok := containers[0].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	_, hasResources := cm["resources"]
	g.Expect(hasResources).Should(BeFalse())
}

func TestMergeObservabilityResourcesOverride(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	source := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "opentelemetry.io/v1beta1",
			"kind":       "OpenTelemetryCollector",
			"metadata":   map[string]any{"name": "collector", "namespace": "ns"},
			"spec": map[string]any{
				"resources": map[string]any{
					"requests": map[string]any{"cpu": "500m", "memory": "1Gi"},
					"limits":   map[string]any{"cpu": "2", "memory": "2Gi"},
				},
			},
		},
	}

	target := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "opentelemetry.io/v1beta1",
			"kind":       "OpenTelemetryCollector",
			"metadata":   map[string]any{"name": "collector", "namespace": "ns"},
			"spec": map[string]any{
				"resources": map[string]any{
					"requests": map[string]any{"cpu": "100m", "memory": "256Mi"},
				},
			},
		},
	}

	err := deploy.MergeObservabilityResources(source, target)
	g.Expect(err).ShouldNot(HaveOccurred())

	cpu, _, err := unstructured.NestedString(target.Object, "spec", "resources", "requests", "cpu")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(cpu).Should(Equal("500m"))
}

func TestMergeObservabilityResourcesNoSourceResources(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	source := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "tempo.grafana.com/v1alpha1",
			"kind":       "TempoStack",
			"metadata":   map[string]any{"name": "ts"},
			"spec":       map[string]any{"storage": map[string]any{"type": "s3"}},
		},
	}

	target := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "tempo.grafana.com/v1alpha1",
			"kind":       "TempoStack",
			"metadata":   map[string]any{"name": "ts"},
			"spec": map[string]any{
				"resources": map[string]any{
					"requests": map[string]any{"cpu": "100m"},
				},
			},
		},
	}

	err := deploy.MergeObservabilityResources(source, target)
	g.Expect(err).ShouldNot(HaveOccurred())

	cpu, _, err := unstructured.NestedString(target.Object, "spec", "resources", "requests", "cpu")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(cpu).Should(Equal("100m"))
}

func TestRemoveDeploymentResources(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := toUnstructured(g, &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("3"),
								corev1.ResourceMemory: resource.MustParse("3Gi"),
							},
						},
					}},
				},
			},
		},
	})

	err := deploy.RemoveDeploymentResources(obj)
	g.Expect(err).ShouldNot(HaveOccurred())

	_, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(found).Should(BeFalse())

	containers, _, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(containers).Should(HaveLen(1))
	cm, _ := containers[0].(map[string]any)
	_, hasResources := cm["resources"]
	g.Expect(hasResources).Should(BeFalse())
}

func deploymentFixture(g Gomega, replicas *int32, cpu, mem string) *unstructured.Unstructured {
	d := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "test",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(cpu),
								corev1.ResourceMemory: resource.MustParse(mem),
							},
						},
					}},
				},
			},
		},
	}

	return toUnstructured(g, d)
}

func toUnstructured(g Gomega, obj any) *unstructured.Unstructured {
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	g.Expect(err).ShouldNot(HaveOccurred())

	return &unstructured.Unstructured{Object: data}
}
