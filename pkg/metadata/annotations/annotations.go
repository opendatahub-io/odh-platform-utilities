// Package annotations provides well-known annotation key constants for the
// ODH platform. Annotations are divided into two categories:
//
//   - Contract annotations: read or written by the orchestrator. Incorrect
//     values may break orchestrator management-state relay or resource
//     lifecycle.
//   - Recommended standard annotations: the blessed convention for the
//     deploy/GC annotation lifecycle. Deploy stamps them, GC reads them to
//     detect stale resources. Module teams not using the shared deploy/GC
//     framework do not need these, but they are the documented standard.
package annotations

import "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"

// --- Contract annotations (orchestrator reads/writes these) ---

const (
	// ManagementStateAnnotation is set by the orchestrator on module CRs to
	// relay the DSC management state (Managed / Removed).
	//
	// Contract: the orchestrator writes this annotation; module controllers
	// must read it to determine their management state.
	ManagementStateAnnotation = "component.opendatahub.io/management-state"

	// ManagedByODHOperator is the resource opt-out convention. Setting this
	// annotation to "false" means the resource is create-only: it will be
	// applied on initial creation but never updated afterward. Both the
	// orchestrator and module controllers should respect this value.
	//
	// Contract: the orchestrator respects this annotation to skip updates.
	ManagedByODHOperator = "opendatahub.io/managed"
)

// --- Recommended standard annotations (deploy/GC lifecycle) ---

const (
	// PlatformVersion records the release version at deploy time. The GC
	// action compares this value to detect stale resources left over from
	// previous releases.
	//
	// Recommended standard: part of the deploy/GC annotation protocol.
	PlatformVersion = labels.ODHPlatformPrefix + "/version"

	// PlatformType records the platform type at deploy time. The GC action
	// checks for platform-type changes.
	//
	// Recommended standard: part of the deploy/GC annotation protocol.
	PlatformType = labels.ODHPlatformPrefix + "/type"

	// InstanceGeneration records the controller CR's .metadata.generation at
	// deploy time. The GC action uses this to detect resources from a
	// previous generation.
	//
	// Recommended standard: part of the deploy/GC annotation protocol.
	InstanceGeneration = labels.ODHPlatformPrefix + "/instance.generation"

	// InstanceName records the controller CR's name. The reconciler builder's
	// default Watches handler maps events back to the owning CR via this
	// annotation.
	//
	// Recommended standard: part of the deploy/GC annotation protocol.
	InstanceName = labels.ODHPlatformPrefix + "/instance.name"

	// InstanceNamespace records the controller CR's namespace. Used together
	// with InstanceName by the dynamic ownership handler to map child
	// resource events back to the owning CR. Empty for cluster-scoped CRs.
	//
	// Recommended standard: part of the deploy/GC annotation protocol.
	InstanceNamespace = labels.ODHPlatformPrefix + "/instance.namespace"

	// InstanceUID records the controller CR's UID. The GC action uses this
	// to detect resources from a deleted-and-recreated CR.
	//
	// Recommended standard: part of the deploy/GC annotation protocol.
	InstanceUID = labels.ODHPlatformPrefix + "/instance.uid"
)
