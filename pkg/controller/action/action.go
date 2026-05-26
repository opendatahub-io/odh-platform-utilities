package action

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/deploy"
)

// Fn is the action-pipeline function signature. Each step in a reconciliation
// pipeline receives a context and the shared ReconciliationRequest, performs
// its work (render, deploy, GC, status update, etc.), and returns an error
// on failure.
//
// Deploy and GC packages produce standalone functions that can be called
// from within an Fn closure. The render package uses its own Fn type
// (render.Fn) with a separate ReconciliationRequest; wrap render calls
// in an action.Fn closure to use them in a pipeline (see package doc).
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

	// Deployer is the stateful deployer used to apply rendered resources
	// to the cluster. It is created once at controller setup and reused
	// across reconciliation cycles. Pipeline deploy actions read this
	// field; render-only pipelines may leave it nil.
	Deployer *deploy.Deployer

	// Conditions is the condition manager for the current reconciliation.
	// Pipeline steps use it to report status (e.g. ProvisioningSucceeded)
	// without needing to manage condition aggregation themselves.
	Conditions *conditions.Manager

	// Resources accumulates rendered Kubernetes resources as they flow
	// through the pipeline. Render actions append here; deploy and GC
	// actions read from here.
	Resources []unstructured.Unstructured
}
