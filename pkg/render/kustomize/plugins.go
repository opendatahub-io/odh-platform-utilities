package kustomize

import (
	"sigs.k8s.io/kustomize/api/builtins" //nolint:staticcheck
	"sigs.k8s.io/kustomize/api/filters/namespace"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/resid"
)

// createNamespaceApplierPlugin creates a plugin to ensure resources have the
// specified target namespace. It handles standard resources, RBAC subject
// namespaces, and webhook configurations.
func createNamespaceApplierPlugin(targetNamespace string) *builtins.NamespaceTransformerPlugin {
	return &builtins.NamespaceTransformerPlugin{
		ObjectMeta: types.ObjectMeta{
			Name:      "odh-namespace-plugin",
			Namespace: targetNamespace,
		},
		FieldSpecs: []types.FieldSpec{
			{
				Gvk:                resid.Gvk{},
				Path:               "metadata/namespace",
				CreateIfNotPresent: true,
			},
			{
				Gvk: resid.Gvk{
					Group: "rbac.authorization.k8s.io",
					Kind:  "ClusterRoleBinding",
				},
				Path:               "subjects/namespace",
				CreateIfNotPresent: true,
			},
			{
				Gvk: resid.Gvk{
					Group: "rbac.authorization.k8s.io",
					Kind:  "RoleBinding",
				},
				Path:               "subjects/namespace",
				CreateIfNotPresent: true,
			},
			{
				Gvk: resid.Gvk{
					Group: "admissionregistration.k8s.io",
					Kind:  "ValidatingWebhookConfiguration",
				},
				Path:               "webhooks/clientConfig/service/namespace",
				CreateIfNotPresent: false,
			},
			{
				Gvk: resid.Gvk{
					Group: "admissionregistration.k8s.io",
					Kind:  "MutatingWebhookConfiguration",
				},
				Path:               "webhooks/clientConfig/service/namespace",
				CreateIfNotPresent: false,
			},
			{
				Gvk: resid.Gvk{
					Group: "apiextensions.k8s.io",
					Kind:  "CustomResourceDefinition",
				},
				Path:               "spec/conversion/webhook/clientConfig/service/namespace",
				CreateIfNotPresent: false,
			},
		},
		UnsetOnly:              false,
		SetRoleBindingSubjects: namespace.AllServiceAccountSubjects,
	}
}

// createSetLabelsPlugin creates a label transformer plugin that applies the
// given labels to metadata/labels and, for Deployments, to
// spec/template/metadata/labels and spec/selector/matchLabels.
func createSetLabelsPlugin(labels map[string]string) *builtins.LabelTransformerPlugin {
	return &builtins.LabelTransformerPlugin{
		Labels: labels,
		FieldSpecs: []types.FieldSpec{
			{
				Gvk:                resid.Gvk{Kind: "Deployment"},
				Path:               "spec/template/metadata/labels",
				CreateIfNotPresent: true,
			},
			{
				Gvk:                resid.Gvk{Kind: "Deployment"},
				Path:               "spec/selector/matchLabels",
				CreateIfNotPresent: true,
			},
			{
				Gvk:                resid.Gvk{},
				Path:               "metadata/labels",
				CreateIfNotPresent: true,
			},
		},
	}
}

// createAddAnnotationsPlugin creates an annotation transformer plugin that
// applies the given annotations to metadata/annotations on all resources.
func createAddAnnotationsPlugin(annotations map[string]string) *builtins.AnnotationsTransformerPlugin {
	return &builtins.AnnotationsTransformerPlugin{
		Annotations: annotations,
		FieldSpecs: []types.FieldSpec{
			{
				Gvk:                resid.Gvk{},
				Path:               "metadata/annotations",
				CreateIfNotPresent: true,
			},
		},
	}
}
