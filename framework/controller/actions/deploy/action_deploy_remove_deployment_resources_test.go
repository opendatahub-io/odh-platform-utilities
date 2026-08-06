package deploy_test

import (
	"testing"

	"github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"

	. "github.com/onsi/gomega"
)

func TestRemoveDeploymentsResources(t *testing.T) {
	g := NewWithT(t)

	source, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("3"),
									corev1.ResourceMemory: resource.MustParse("3Gi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
						},
					},
				},
			},
		},
	})
	g.Expect(err).ShouldNot(HaveOccurred())

	src := unstructured.Unstructured{Object: source}

	err = deploy.RemoveDeploymentsResources(&src)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(src).Should(And(
		jq.Match(`.spec | has("replicas") | not`),
		jq.Match(`.spec.template.spec.containers[0] | has("resources") | not`),
	))
}
