// Package precondition provides a composable framework for gating
// reconciliation on external dependencies such as CRD presence, API
// availability, or cluster capabilities.
//
// # Core Concepts
//
// A [PreCondition] wraps a [CheckFunc] with framework configuration
// (condition type, severity, cluster filtering, stop semantics). [RunAll]
// executes all preconditions, aggregates results per condition type
// (False > Unknown > True priority), and writes Kubernetes status
// conditions via the conditions manager on the [action.ReconciliationRequest].
//
// # Standalone Usage
//
// The package does not require the reconciler builder. Construct a
// [action.ReconciliationRequest] manually and call [RunAll]:
//
//	rr := &action.ReconciliationRequest{
//	    Client:     cli,
//	    Instance:   myCR,
//	    Conditions: conditions.NewManager(myCR,
//	        string(common.ConditionTypeReady),
//	        precondition.ConditionTypeDependenciesAvailable,
//	    ),
//	}
//
//	pcs := []precondition.PreCondition{
//	    precondition.MonitorCRD(schema.GroupVersionKind{
//	        Group:   "serving.knative.dev",
//	        Version: "v1",
//	        Kind:    "Service",
//	    }, precondition.WithStopReconciliation()),
//	}
//
//	clusterType, err := cluster.DetectClusterType(ctx, cli)
//	if err != nil {
//	    return ctrl.Result{}, fmt.Errorf("detect cluster type: %w", err)
//	}
//
//	if precondition.RunAll(ctx, rr, clusterType, pcs) {
//	    // A critical dependency is missing — skip reconciliation.
//	    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
//	}
//
// The clusterType parameter controls [WithClusterTypes] filtering.
// Pass an empty string to disable filtering when none of your
// preconditions use [WithClusterTypes].
//
// # Built-In Checks
//
// [MonitorCRD] and [MonitorCRDs] check for CRD presence via the
// client's RESTMapper (single-shot, no retry).
// [Custom] wraps any caller-provided [CheckFunc].
//
// # Functional Options
//
//   - [WithConditionType] — target a specific status condition
//   - [WithSeverity] — Error (default) or Info
//   - [WithStopReconciliation] — halt reconciliation on failure
//   - [WithClusterTypes] — only run on specific cluster types
//   - [WithMessage] — override the check's failure message
//   - [WithSkipFunc] — runtime predicate to conditionally skip
//
// # Security
//
// Error messages from [CheckFunc] and [SkipFunc] are written to
// controller logs and Kubernetes status conditions. Implementations
// must never include sensitive data (passwords, tokens, API keys,
// Secret.Data) in returned errors or [CheckResult.Message] values.
//
// # Design Note
//
// This package provides a way to do pre-reconciliation checks, not the
// way. Teams that want to check CRD presence manually with
// [cluster.HasCRD] and write conditions by hand are free to do so.
package precondition
