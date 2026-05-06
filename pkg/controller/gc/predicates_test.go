package gc_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/gc"
	odhAnnotations "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
)

func TestDefaultObjectPredicate_NoAnnotations(t *testing.T) {
	t.Parallel()

	params := gc.RunParams{
		Owner:        newFakeOwner(1),
		Version:      "1.0.0",
		PlatformType: "OpenDataHub",
	}

	obj := unstructured.Unstructured{}
	obj.SetName("test-resource")

	deletable, err := gc.DefaultObjectPredicate(params, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deletable {
		t.Error("expected resource with no annotations to not be deletable")
	}
}

func TestDefaultObjectPredicate_MissingAnnotations(t *testing.T) {
	t.Parallel()

	params := gc.RunParams{
		Owner:        newFakeOwner(1),
		Version:      "1.0.0",
		PlatformType: "OpenDataHub",
	}

	obj := unstructured.Unstructured{}
	obj.SetName("test-resource")
	obj.SetAnnotations(map[string]string{
		odhAnnotations.PlatformVersion: "1.0.0",
	})

	deletable, err := gc.DefaultObjectPredicate(params, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deletable {
		t.Error("expected resource with missing lifecycle annotations to be deletable")
	}
}

func TestDefaultObjectPredicate_MatchingAnnotations(t *testing.T) {
	t.Parallel()

	params := gc.RunParams{
		Owner:        newFakeOwner(1),
		Version:      "1.0.0",
		PlatformType: "OpenDataHub",
	}

	obj := unstructured.Unstructured{}
	obj.SetName("test-resource")
	obj.SetAnnotations(map[string]string{
		odhAnnotations.PlatformVersion:    "1.0.0",
		odhAnnotations.PlatformType:       "OpenDataHub",
		odhAnnotations.InstanceGeneration: "1",
		odhAnnotations.InstanceUID:        "uid-1",
	})

	deletable, err := gc.DefaultObjectPredicate(params, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deletable {
		t.Error("expected resource with matching annotations to not be deletable")
	}
}

func TestDefaultObjectPredicate_VersionMismatch(t *testing.T) {
	t.Parallel()

	params := gc.RunParams{
		Owner:        newFakeOwner(1),
		Version:      "2.0.0",
		PlatformType: "OpenDataHub",
	}

	obj := unstructured.Unstructured{}
	obj.SetName("test-resource")
	obj.SetAnnotations(map[string]string{
		odhAnnotations.PlatformVersion:    "1.0.0",
		odhAnnotations.PlatformType:       "OpenDataHub",
		odhAnnotations.InstanceGeneration: "1",
		odhAnnotations.InstanceUID:        "uid-1",
	})

	deletable, err := gc.DefaultObjectPredicate(params, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deletable {
		t.Error("expected resource with version mismatch to be deletable")
	}
}

func TestDefaultObjectPredicate_PlatformTypeMismatch(t *testing.T) {
	t.Parallel()

	params := gc.RunParams{
		Owner:        newFakeOwner(1),
		Version:      "1.0.0",
		PlatformType: "ManagedRhoai",
	}

	obj := unstructured.Unstructured{}
	obj.SetName("test-resource")
	obj.SetAnnotations(map[string]string{
		odhAnnotations.PlatformVersion:    "1.0.0",
		odhAnnotations.PlatformType:       "OpenDataHub",
		odhAnnotations.InstanceGeneration: "1",
		odhAnnotations.InstanceUID:        "uid-1",
	})

	deletable, err := gc.DefaultObjectPredicate(params, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deletable {
		t.Error("expected resource with platform type mismatch to be deletable")
	}
}

func TestDefaultObjectPredicate_UIDMismatch(t *testing.T) {
	t.Parallel()

	params := gc.RunParams{
		Owner:        newFakeOwner(1),
		Version:      "1.0.0",
		PlatformType: "OpenDataHub",
	}

	obj := unstructured.Unstructured{}
	obj.SetName("test-resource")
	obj.SetAnnotations(map[string]string{
		odhAnnotations.PlatformVersion:    "1.0.0",
		odhAnnotations.PlatformType:       "OpenDataHub",
		odhAnnotations.InstanceGeneration: "1",
		odhAnnotations.InstanceUID:        "uid-different",
	})

	deletable, err := gc.DefaultObjectPredicate(params, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deletable {
		t.Error("expected resource with UID mismatch to be deletable")
	}
}

func TestDefaultObjectPredicate_GenerationAdvanced(t *testing.T) {
	t.Parallel()

	params := gc.RunParams{
		Owner:        newFakeOwner(5),
		Version:      "1.0.0",
		PlatformType: "OpenDataHub",
	}

	obj := unstructured.Unstructured{}
	obj.SetName("test-resource")
	obj.SetAnnotations(map[string]string{
		odhAnnotations.PlatformVersion:    "1.0.0",
		odhAnnotations.PlatformType:       "OpenDataHub",
		odhAnnotations.InstanceGeneration: "3",
		odhAnnotations.InstanceUID:        "uid-1",
	})

	deletable, err := gc.DefaultObjectPredicate(params, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deletable {
		t.Error("expected resource with advanced generation to be deletable")
	}
}

func TestDefaultObjectPredicate_GenerationBehind(t *testing.T) {
	t.Parallel()

	params := gc.RunParams{
		Owner:        newFakeOwner(2),
		Version:      "1.0.0",
		PlatformType: "OpenDataHub",
	}

	obj := unstructured.Unstructured{}
	obj.SetName("test-resource")
	obj.SetAnnotations(map[string]string{
		odhAnnotations.PlatformVersion:    "1.0.0",
		odhAnnotations.PlatformType:       "OpenDataHub",
		odhAnnotations.InstanceGeneration: "5",
		odhAnnotations.InstanceUID:        "uid-1",
	})

	deletable, err := gc.DefaultObjectPredicate(params, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deletable {
		t.Error("expected resource with mismatched generation to be deletable")
	}
}

func TestDefaultObjectPredicate_InvalidGeneration(t *testing.T) {
	t.Parallel()

	params := gc.RunParams{
		Owner:        newFakeOwner(1),
		Version:      "1.0.0",
		PlatformType: "OpenDataHub",
	}

	obj := unstructured.Unstructured{}
	obj.SetName("test-resource")
	obj.SetAnnotations(map[string]string{
		odhAnnotations.PlatformVersion:    "1.0.0",
		odhAnnotations.PlatformType:       "OpenDataHub",
		odhAnnotations.InstanceGeneration: "not-a-number",
		odhAnnotations.InstanceUID:        "uid-1",
	})

	_, err := gc.DefaultObjectPredicate(params, obj)
	if err == nil {
		t.Error("expected error for invalid generation annotation")
	}
}

func TestDefaultTypePredicate(t *testing.T) {
	t.Parallel()

	params := gc.RunParams{}

	gvk := schema.GroupVersionKind{
		Group: "coordination.k8s.io", Version: "v1", Kind: "Lease",
	}

	deletable, err := gc.DefaultTypePredicate(params, gvk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deletable {
		t.Error("expected DefaultTypePredicate to return true for any GVK")
	}
}

// fakeOwner implements client.Object for testing.
type fakeOwner struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (f *fakeOwner) DeepCopyObject() runtime.Object { return f }

func newFakeOwner(generation int64) *fakeOwner {
	return &fakeOwner{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-owner",
			Generation: generation,
			UID:        types.UID("uid-1"),
		},
	}
}
