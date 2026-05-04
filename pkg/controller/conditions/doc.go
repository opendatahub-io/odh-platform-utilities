// Package conditions provides condition management utilities for module
// controller status.
//
// The Manager type offers Knative-inspired condition management with automatic
// aggregation of dependent conditions into a happy condition (typically Ready).
// Severity-based filtering controls which conditions participate in rollup.
//
// Pattern 1 — Module controller reconcile:
//
//	mgr := conditions.NewManager(&obj.Status,
//	    string(common.ConditionTypeReady),
//	    string(common.ConditionTypeProvisioningSucceeded),
//	)
//
//	if err := deploy(); err != nil {
//	    mgr.MarkFalse(string(common.ConditionTypeProvisioningSucceeded),
//	        conditions.WithError(err))
//	    return err
//	}
//	mgr.MarkTrue(string(common.ConditionTypeProvisioningSucceeded))
//
// Pattern 2 — Status propagation from a sub-component CR:
//
//	childCond := conditions.FindStatusCondition(childAccessor, "Ready")
//	mgr.MarkFrom("SubcomponentReady", childCond)
//
// The package also exports low-level helpers (SetStatusCondition,
// FindStatusCondition, RemoveStatusCondition) for manual condition management.
//
// See AGENTS.md for detailed usage patterns and examples.
package conditions
