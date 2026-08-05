package resources_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"

	. "github.com/onsi/gomega"
)

func newUnstructured(group, version, kind, namespace, name string) unstructured.Unstructured {
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: version, Kind: kind})
	u.SetNamespace(namespace)
	u.SetName(name)
	return u
}

func TestSortByApplyOrder(t *testing.T) {
	orderingCases := []struct {
		name          string
		input         []unstructured.Unstructured
		expectedKinds []string
	}{
		{
			name: "sorts CRDs before Deployments before unknown kinds",
			input: []unstructured.Unstructured{
				newUnstructured("example.com", "v1", "UnknownKind", "ns", "my-unknown"),
				newUnstructured(gvk.Deployment.Group, gvk.Deployment.Version, gvk.Deployment.Kind, "ns", "my-deploy"),
				newUnstructured(gvk.CustomResourceDefinition.Group, gvk.CustomResourceDefinition.Version, gvk.CustomResourceDefinition.Kind, "", "my-crd"),
			},
			expectedKinds: []string{"CustomResourceDefinition", "Deployment", "UnknownKind"},
		},
		{
			name: "sorts webhooks last",
			input: []unstructured.Unstructured{
				newUnstructured("admissionregistration.k8s.io", "v1", "ValidatingWebhookConfiguration", "", "webhook"),
				newUnstructured(gvk.Namespace.Group, gvk.Namespace.Version, gvk.Namespace.Kind, "", "my-ns"),
				newUnstructured(gvk.Deployment.Group, gvk.Deployment.Version, gvk.Deployment.Kind, "ns", "my-deploy"),
			},
			expectedKinds: []string{"Namespace", "Deployment", "ValidatingWebhookConfiguration"},
		},
	}

	for _, tc := range orderingCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			result, err := resources.SortByApplyOrder(context.Background(), tc.input)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(result).To(HaveLen(len(tc.expectedKinds)))
			for i, expected := range tc.expectedKinds {
				g.Expect(result[i].GetKind()).To(Equal(expected))
			}
		})
	}

	t.Run("unknown kinds placed in middle", func(t *testing.T) {
		g := NewWithT(t)

		input := []unstructured.Unstructured{
			newUnstructured("admissionregistration.k8s.io", "v1", "ValidatingWebhookConfiguration", "", "webhook"),
			newUnstructured("example.com", "v1", "UnknownKind", "ns", "my-unknown"),
			newUnstructured(gvk.Namespace.Group, gvk.Namespace.Version, gvk.Namespace.Kind, "", "my-ns"),
		}

		result, err := resources.SortByApplyOrder(context.Background(), input)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(HaveLen(3))
		g.Expect(result[0].GetKind()).To(Equal("Namespace"))
		g.Expect(result[1].GetKind()).To(Equal("UnknownKind"))
		g.Expect(result[2].GetKind()).To(Equal("ValidatingWebhookConfiguration"))
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		g := NewWithT(t)

		result, err := resources.SortByApplyOrder(context.Background(), nil)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(BeEmpty())
	})

	t.Run("stable sort preserves order for same kind", func(t *testing.T) {
		g := NewWithT(t)

		input := []unstructured.Unstructured{
			newUnstructured(gvk.Deployment.Group, gvk.Deployment.Version, gvk.Deployment.Kind, "ns", "deploy-b"),
			newUnstructured(gvk.Deployment.Group, gvk.Deployment.Version, gvk.Deployment.Kind, "ns", "deploy-a"),
		}

		result, err := resources.SortByApplyOrder(context.Background(), input)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(HaveLen(2))
		g.Expect(result[0].GetName()).To(Equal("deploy-a"))
		g.Expect(result[1].GetName()).To(Equal("deploy-b"))
	})

	workloadTestCases := []struct {
		name         string
		group        string
		version      string
		kind         string
		resourceName string
	}{
		{"deployments", gvk.Deployment.Group, gvk.Deployment.Version, gvk.Deployment.Kind, "consuming-app"},
		{"statefulsets", gvk.StatefulSet.Group, gvk.StatefulSet.Version, gvk.StatefulSet.Kind, "consuming-statefulset"},
		{"daemonsets", "apps", "v1", "DaemonSet", "consuming-daemonset"},
		{"jobs", "batch", "v1", "Job", "consuming-job"},
	}

	for _, tc := range workloadTestCases {
		t.Run("cert-manager resources placed before "+tc.name, func(t *testing.T) {
			testCertManagerOrderingForWorkload(t, tc.group, tc.version, tc.kind, tc.resourceName)
		})
	}

	t.Run("comprehensive cert-manager dependency ordering", func(t *testing.T) {
		g := NewWithT(t)

		input := []unstructured.Unstructured{
			newUnstructured(gvk.Deployment.Group, gvk.Deployment.Version, gvk.Deployment.Kind, "app-ns", "consuming-app"),
			newUnstructured("admissionregistration.k8s.io", "v1", "ValidatingWebhookConfiguration", "", "webhook"),
			newUnstructured(gvk.CertManagerCertificate.Group, gvk.CertManagerCertificate.Version, gvk.CertManagerCertificate.Kind, "cert-manager", "ca-cert"),
			newUnstructured(gvk.Service.Group, gvk.Service.Version, gvk.Service.Kind, "app-ns", "app-service"),
			newUnstructured(gvk.CertManagerClusterIssuer.Group, gvk.CertManagerClusterIssuer.Version, gvk.CertManagerClusterIssuer.Kind, "", "ca-issuer"),
			newUnstructured(gvk.Namespace.Group, gvk.Namespace.Version, gvk.Namespace.Kind, "", "app-ns"),
			newUnstructured(gvk.CertManagerIssuer.Group, gvk.CertManagerIssuer.Version, gvk.CertManagerIssuer.Kind, "cert-manager", "self-signed-issuer"),
			newUnstructured(gvk.CustomResourceDefinition.Group, gvk.CustomResourceDefinition.Version, gvk.CustomResourceDefinition.Kind, "", "certificates.cert-manager.io"),
		}

		result, err := resources.SortByApplyOrder(context.Background(), input)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(HaveLen(8))

		g.Expect(result[0].GetKind()).To(Equal("Namespace"))
		g.Expect(result[1].GetKind()).To(Equal("CustomResourceDefinition"))
		g.Expect(result[2].GetKind()).To(Equal("Service"))
		g.Expect(result[3].GetKind()).To(Equal("ClusterIssuer"))
		g.Expect(result[4].GetKind()).To(Equal("Issuer"))
		g.Expect(result[5].GetKind()).To(Equal("Certificate"))
		g.Expect(result[6].GetKind()).To(Equal("Deployment"))
		g.Expect(result[7].GetKind()).To(Equal("ValidatingWebhookConfiguration"))
	})
}

func testCertManagerOrderingForWorkload(t *testing.T, group, version, kind, resourceName string) {
	t.Helper()
	g := NewWithT(t)

	input := []unstructured.Unstructured{
		newUnstructured(group, version, kind, "app-ns", resourceName),
		newUnstructured(gvk.CertManagerCertificate.Group, gvk.CertManagerCertificate.Version, gvk.CertManagerCertificate.Kind, "cert-manager", "ca-cert"),
		newUnstructured(gvk.CertManagerClusterIssuer.Group, gvk.CertManagerClusterIssuer.Version, gvk.CertManagerClusterIssuer.Kind, "", "ca-issuer"),
		newUnstructured(gvk.Namespace.Group, gvk.Namespace.Version, gvk.Namespace.Kind, "", "app-ns"),
	}

	result, err := resources.SortByApplyOrder(context.Background(), input)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(HaveLen(4))

	clusterIssuerIndex := -1
	certificateIndex := -1
	workloadIndex := -1

	for i, resource := range result {
		switch resource.GetKind() {
		case gvk.CertManagerClusterIssuer.Kind:
			clusterIssuerIndex = i
		case gvk.CertManagerCertificate.Kind:
			certificateIndex = i
		case kind:
			workloadIndex = i
		}
	}

	g.Expect(clusterIssuerIndex).To(BeNumerically("<", workloadIndex),
		"ClusterIssuer must be deployed before %s to prevent transient errors", kind)
	g.Expect(certificateIndex).To(BeNumerically("<", workloadIndex),
		"Certificate must be deployed before %s to prevent transient errors", kind)

	g.Expect(clusterIssuerIndex).To(BeNumerically("<", certificateIndex),
		"ClusterIssuer must be deployed before Certificate")

	g.Expect(result[0].GetKind()).To(Equal("Namespace"))
	g.Expect(result[1].GetKind()).To(Equal("ClusterIssuer"))
	g.Expect(result[2].GetKind()).To(Equal("Certificate"))
	g.Expect(result[3].GetKind()).To(Equal(kind))
}

func TestIsWebhookResource(t *testing.T) {
	t.Run("correctly identifies webhook resources", func(t *testing.T) {
		g := NewWithT(t)

		mutatingWebhook := newUnstructured(gvk.MutatingWebhookConfiguration.Group, gvk.MutatingWebhookConfiguration.Version, gvk.MutatingWebhookConfiguration.Kind, "", "test-mutating")
		g.Expect(resources.IsWebhookResource(mutatingWebhook)).To(BeTrue(), "MutatingWebhookConfiguration should be identified as webhook resource")

		validatingWebhook := newUnstructured(
			gvk.ValidatingWebhookConfiguration.Group,
			gvk.ValidatingWebhookConfiguration.Version,
			gvk.ValidatingWebhookConfiguration.Kind,
			"",
			"test-validating",
		)
		g.Expect(resources.IsWebhookResource(validatingWebhook)).To(BeTrue(), "ValidatingWebhookConfiguration should be identified as webhook resource")

		admissionPolicy := newUnstructured(
			gvk.ValidatingAdmissionPolicy.Group,
			gvk.ValidatingAdmissionPolicy.Version,
			gvk.ValidatingAdmissionPolicy.Kind,
			"",
			"test-policy",
		)
		g.Expect(resources.IsWebhookResource(admissionPolicy)).To(BeTrue(), "ValidatingAdmissionPolicy should be identified as webhook resource")

		policyBinding := newUnstructured(
			gvk.ValidatingAdmissionPolicyBinding.Group,
			gvk.ValidatingAdmissionPolicyBinding.Version,
			gvk.ValidatingAdmissionPolicyBinding.Kind,
			"",
			"test-binding",
		)
		g.Expect(resources.IsWebhookResource(policyBinding)).To(BeTrue(), "ValidatingAdmissionPolicyBinding should be identified as webhook resource")

		deployment := newUnstructured(gvk.Deployment.Group, gvk.Deployment.Version, gvk.Deployment.Kind, "ns", "test-deployment")
		g.Expect(resources.IsWebhookResource(deployment)).To(BeFalse(), "Deployment should not be identified as webhook resource")

		service := newUnstructured(gvk.Service.Group, gvk.Service.Version, gvk.Service.Kind, "ns", "test-service")
		g.Expect(resources.IsWebhookResource(service)).To(BeFalse(), "Service should not be identified as webhook resource")
	})
}

func TestCertManagerResourcesPlacedBeforeWebhooks(t *testing.T) {
	webhookTestCases := []struct {
		name         string
		group        string
		version      string
		kind         string
		resourceName string
	}{
		{
			"MutatingWebhookConfiguration",
			gvk.MutatingWebhookConfiguration.Group,
			gvk.MutatingWebhookConfiguration.Version,
			gvk.MutatingWebhookConfiguration.Kind,
			"test-mutating",
		},
		{
			"ValidatingWebhookConfiguration",
			gvk.ValidatingWebhookConfiguration.Group,
			gvk.ValidatingWebhookConfiguration.Version,
			gvk.ValidatingWebhookConfiguration.Kind,
			"test-validating",
		},
		{
			"ValidatingAdmissionPolicy",
			gvk.ValidatingAdmissionPolicy.Group,
			gvk.ValidatingAdmissionPolicy.Version,
			gvk.ValidatingAdmissionPolicy.Kind,
			"test-policy",
		},
		{
			"ValidatingAdmissionPolicyBinding",
			gvk.ValidatingAdmissionPolicyBinding.Group,
			gvk.ValidatingAdmissionPolicyBinding.Version,
			gvk.ValidatingAdmissionPolicyBinding.Kind,
			"test-binding",
		},
	}

	for _, tc := range webhookTestCases {
		t.Run("cert-manager resources placed before "+tc.name, func(t *testing.T) {
			g := NewWithT(t)

			input := []unstructured.Unstructured{
				newUnstructured(tc.group, tc.version, tc.kind, "", tc.resourceName),
				newUnstructured(gvk.CertManagerCertificate.Group, gvk.CertManagerCertificate.Version, gvk.CertManagerCertificate.Kind, "cert-manager", "ca-cert"),
				newUnstructured(gvk.CertManagerClusterIssuer.Group, gvk.CertManagerClusterIssuer.Version, gvk.CertManagerClusterIssuer.Kind, "", "ca-issuer"),
				newUnstructured(gvk.Namespace.Group, gvk.Namespace.Version, gvk.Namespace.Kind, "", "test-ns"),
			}

			result, err := resources.SortByApplyOrder(context.Background(), input)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(result).To(HaveLen(4))

			clusterIssuerIndex := -1
			certificateIndex := -1
			webhookIndex := -1

			for i, resource := range result {
				switch resource.GetKind() {
				case gvk.CertManagerClusterIssuer.Kind:
					clusterIssuerIndex = i
				case gvk.CertManagerCertificate.Kind:
					certificateIndex = i
				case tc.kind:
					webhookIndex = i
				}
			}

			g.Expect(clusterIssuerIndex).To(BeNumerically("<", webhookIndex),
				"ClusterIssuer must be deployed before %s", tc.kind)
			g.Expect(certificateIndex).To(BeNumerically("<", webhookIndex),
				"Certificate must be deployed before %s", tc.kind)

			g.Expect(clusterIssuerIndex).To(BeNumerically("<", certificateIndex),
				"ClusterIssuer must be deployed before Certificate")

			g.Expect(result[0].GetKind()).To(Equal("Namespace"))
			g.Expect(result[1].GetKind()).To(Equal("ClusterIssuer"))
			g.Expect(result[2].GetKind()).To(Equal("Certificate"))
			g.Expect(result[3].GetKind()).To(Equal(tc.kind))
		})
	}
}
