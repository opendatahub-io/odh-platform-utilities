package action

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
)

// Fn is the action-pipeline function signature. Each step in a reconciliation
// pipeline receives a context and the shared ReconciliationRequest, performs
// its work (render, deploy, GC, status update, etc.), and returns an error
// on failure.
//
// Deploy, GC, and render packages produce standalone functions compatible
// with this signature. Teams that don't want the pipeline can call those
// functions directly.
type Fn func(ctx context.Context, rr *ReconciliationRequest) error

// ReconciliationRequest carries the shared state for an action pipeline.
// It is passed between pipeline steps, allowing each step to read inputs
// and write outputs consumed by subsequent steps.
//
// The struct can be instantiated manually by module teams that want
// fine-grained control, or populated by a reconciler builder for
// convention-based usage.
type ReconciliationRequest struct {
	// Client is the Kubernetes API client for cluster operations.
	Client client.Client

	// Instance is the module CR being reconciled. It implements
	// PlatformObject, giving pipeline steps access to metadata, status,
	// conditions, and release tracking.
	Instance common.PlatformObject

	// Conditions is the condition manager for the current reconciliation.
	// Pipeline steps use it to report status (e.g. ProvisioningSucceeded)
	// without needing to manage condition aggregation themselves.
	Conditions *conditions.Manager

	// Resources accumulates rendered Kubernetes resources as they flow
	// through the pipeline. Render actions append here; deploy and GC
	// actions read from here.
	Resources []unstructured.Unstructured
}
