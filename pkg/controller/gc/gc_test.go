package gc_test

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/gc"
)

func TestNew_Defaults(t *testing.T) {
	t.Parallel()

	c := gc.New()
	if c == nil {
		t.Fatal("expected New() to return a non-nil Collector")
	}
}

func TestNew_WithLabel(t *testing.T) {
	t.Parallel()

	c := gc.New(gc.WithLabel("app", "test"))
	if c == nil {
		t.Fatal("expected New() to return a non-nil Collector")
	}
}

func TestNew_WithLabels(t *testing.T) {
	t.Parallel()

	c := gc.New(gc.WithLabels(map[string]string{"a": "1", "b": "2"}))
	if c == nil {
		t.Fatal("expected New() to return a non-nil Collector")
	}
}

func TestNew_WithUnremovables(t *testing.T) {
	t.Parallel()

	extra := schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "Foo"}
	c := gc.New(gc.WithUnremovables(extra))

	if c == nil {
		t.Fatal("expected New() to return a non-nil Collector")
	}
}

func TestNew_WithOnlyCollectOwned(t *testing.T) {
	t.Parallel()

	c := gc.New(gc.WithOnlyCollectOwned(false))
	if c == nil {
		t.Fatal("expected New() to return a non-nil Collector")
	}
}

func TestNew_WithDeletePropagationPolicy(t *testing.T) {
	t.Parallel()

	c := gc.New(gc.WithDeletePropagationPolicy(metav1.DeletePropagationBackground))
	if c == nil {
		t.Fatal("expected New() to return a non-nil Collector")
	}
}

func TestNew_WithNilObjectPredicate(t *testing.T) {
	t.Parallel()

	c := gc.New(gc.WithObjectPredicate(nil))
	if c == nil {
		t.Fatal("expected nil predicate to be ignored, returning valid Collector")
	}
}

func TestNew_WithNilTypePredicate(t *testing.T) {
	t.Parallel()

	c := gc.New(gc.WithTypePredicate(nil))
	if c == nil {
		t.Fatal("expected nil predicate to be ignored, returning valid Collector")
	}
}

func TestNew_WithMetrics(t *testing.T) {
	t.Parallel()

	c := gc.New(gc.WithMetrics())
	if c == nil {
		t.Fatal("expected New() with WithMetrics to return a non-nil Collector")
	}
}

func TestNew_InNamespace(t *testing.T) {
	t.Parallel()

	c := gc.New(gc.InNamespace("test-ns"))
	if c == nil {
		t.Fatal("expected New() to return a non-nil Collector")
	}
}

func TestNew_InNamespaceFn_Nil(t *testing.T) {
	t.Parallel()

	c := gc.New(gc.InNamespaceFn(nil))
	if c == nil {
		t.Fatal("expected nil fn to be ignored, returning valid Collector")
	}
}

func TestRun_NilParams(t *testing.T) {
	t.Parallel()

	c := gc.New()

	err := c.Run(context.TODO(), gc.RunParams{})
	if err == nil {
		t.Error("expected error for nil RunParams fields")
	}
}

func TestRun_NilClient(t *testing.T) {
	t.Parallel()

	c := gc.New()

	err := c.Run(context.TODO(), gc.RunParams{
		Owner: newFakeOwner(1),
	})
	if err == nil {
		t.Error("expected error when Client is nil")
	}
}

func TestRun_NilOwner(t *testing.T) {
	t.Parallel()

	c := gc.New()

	err := c.Run(context.TODO(), gc.RunParams{
		Client: fakeClient{},
	})
	if err == nil {
		t.Error("expected error when Owner is nil")
	}
}

// fakeClient satisfies the client.Client interface check in RunParams
// validation but is not a real client.
type fakeClient struct {
	client.Client
}
