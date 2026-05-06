// Package deploy provides utilities for applying rendered Kubernetes manifests
// to the cluster with correct ownership, field management, partial
// modification support, and ordering.
//
// This is a convenience package — module teams are free to use raw
// client.Patch with SSA or any other deployment mechanism. The utilities here
// encode patterns (deployment merging, caching, annotation-based opt-out,
// apply ordering) that reduce boilerplate and ensure consistency across
// modules.
//
// # Quick Start
//
//	deployer := deploy.NewDeployer(
//	    deploy.WithFieldOwner("mycontroller"),
//	    deploy.WithMode(deploy.ModeSSA),
//	    deploy.WithApplyOrder(),
//	    deploy.WithCache(),
//	    deploy.WithMergeStrategy(deploymentGVK, deploy.MergeDeployments),
//	)
//
//	err := deployer.Deploy(ctx, deploy.DeployInput{
//	    Client:    cli,
//	    Resources: renderedResources,
//	    Owner:     myCR,
//	    Release:   deploy.ReleaseInfo{Type: "OpenDataHub", Version: "2.0.0"},
//	})
//
// # Merge Strategies
//
// Merge strategies preserve user-customised fields during SSA apply.
// Register them per GVK via [WithMergeStrategy]:
//
//   - [MergeDeployments]: preserves container resources and replicas.
//   - [MergeObservabilityResources]: preserves spec.resources for
//     monitoring-stack types.
//
// Module teams can implement custom [MergeFn] functions for their own types.
//
// # Deploy Caching
//
// [Cache] is a TTL-based store that fingerprints each
// (live-resource, desired-manifest) pair. On subsequent reconciliation loops,
// if neither the live resource nor the desired manifest has changed, the
// deploy is skipped. Enable caching with [WithCache].
//
// # Annotation Conventions
//
// The deployer stamps platform annotations on every resource (instance
// generation, name, UID, platform type, version) to support garbage
// collection. The "opendatahub.io/managed" annotation is respected as a
// create-only opt-out: resources with managed=false are created but never
// updated.
package deploy
