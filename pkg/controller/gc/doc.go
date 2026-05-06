// Package gc provides garbage collection utilities for Kubernetes module
// controllers. GC handles cleanup of resources that are no longer desired
// after a configuration change, upgrade, or component removal.
//
// The Collector compares what is currently deployed on the cluster (discovered
// via the Kubernetes API) against what the controller just rendered, and
// deletes the difference. It uses:
//   - API discovery to enumerate all resource types on the cluster
//   - RBAC authorization checks to filter to types the controller can delete
//   - Label-based selection to scope to resources the controller owns
//   - Two-level predicate filtering (type-level and object-level)
//   - Configurable unremovable types, deletion propagation, and namespace scoping
//
// Basic usage:
//
//	collector := gc.New(
//	    gc.InNamespace("my-namespace"),
//	)
//
//	if generated {
//	    err := collector.Run(ctx, gc.RunParams{
//	        Client:          cli,
//	        DynamicClient:   dynamicCli,
//	        DiscoveryClient: discoveryCli,
//	        Owner:           myCR,
//	        Version:         "1.2.0",
//	        PlatformType:    "OpenDataHub",
//	    })
//	}
//
// GC should be the last action in a reconcile cycle, after all deploy actions
// have completed. Callers should skip GC when nothing was generated to avoid
// expensive API discovery on no-op reconciles.
//
// See AGENTS.md for detailed usage patterns and examples.
package gc
