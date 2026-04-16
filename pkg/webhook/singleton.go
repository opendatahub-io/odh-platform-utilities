package webhook

import (
	"context"
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ValidateSingletonCreation is a validating admission webhook helper that
// denies object creation when another instance of the given GVK already
// exists in the cluster. For non-CREATE operations the request is allowed
// unconditionally.
func ValidateSingletonCreation(
	ctx context.Context,
	r client.Reader,
	req *admission.Request,
	gvk schema.GroupVersionKind,
) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("singleton validation only applies to CREATE operations")
	}

	count, err := CountObjects(ctx, r, gvk)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return DenyCountGtZero(count, gvk)
}

// CountObjects returns the number of objects of the given GVK that currently
// exist in the cluster. It uses an unstructured list so no typed ObjectList
// registration is required.
func CountObjects(
	ctx context.Context,
	r client.Reader,
	gvk schema.GroupVersionKind,
) (int, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind + "List",
	})

	err := r.List(ctx, list)
	if err != nil {
		return 0, fmt.Errorf("counting %s objects: %w", gvk.Kind, err)
	}

	return len(list.Items), nil
}

// DenyCountGtZero returns a deny response if count is greater than zero,
// indicating that an instance already exists and the singleton constraint
// would be violated. Otherwise it returns an allow response.
func DenyCountGtZero(count int, gvk schema.GroupVersionKind) admission.Response {
	if count > 0 {
		return admission.Denied(fmt.Sprintf(
			"only one instance of %s is allowed; an instance already exists",
			gvk.Kind,
		))
	}

	return admission.Allowed("")
}
