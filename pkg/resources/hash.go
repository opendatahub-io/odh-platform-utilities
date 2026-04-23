package resources

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Hash generates an SHA-256 hash of an unstructured Kubernetes object, omitting
// server-managed fields (uid, resourceVersion, deletionTimestamp, managedFields,
// ownerReferences, and the status subresource). The hash is suitable for
// detecting meaningful content changes between reconciliation loops.
func Hash(in *unstructured.Unstructured) ([]byte, error) {
	obj := in.DeepCopy()
	unstructured.RemoveNestedField(obj.Object, "metadata", "uid")
	unstructured.RemoveNestedField(obj.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(obj.Object, "metadata", "deletionTimestamp")
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(obj.Object, "metadata", "ownerReferences")
	unstructured.RemoveNestedField(obj.Object, "status")

	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}

	hasher := sha256.New()

	_, err := printer.Fprintf(hasher, "%#v", obj)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate hash: %w", err)
	}

	return hasher.Sum(nil), nil
}

// EncodeToString returns a URL-safe base64 encoding of in, prefixed with "v".
func EncodeToString(in []byte) string {
	return "v" + base64.RawURLEncoding.EncodeToString(in)
}

// StripServerMetadata removes server-managed metadata fields from a resource,
// returning a clean copy suitable for creation, comparison, or backup.
func StripServerMetadata(obj *unstructured.Unstructured) *unstructured.Unstructured {
	if obj == nil {
		return nil
	}

	clean := obj.DeepCopy()

	unstructured.RemoveNestedField(clean.Object, "metadata", "uid")
	unstructured.RemoveNestedField(clean.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(clean.Object, "metadata", "generation")
	unstructured.RemoveNestedField(clean.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(clean.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(clean.Object, "metadata", "deletionTimestamp")
	unstructured.RemoveNestedField(clean.Object, "metadata", "ownerReferences")
	unstructured.RemoveNestedField(clean.Object, "status")

	return clean
}
