# Metadata Conventions

This package defines the well-known labels and annotations that form the
contract and recommended standards between the ODH orchestrator, module
controllers, and deployed resources.

## Contract vs Recommended Standard

**Contract** items are required by the orchestrator. If they are missing or
incorrect, the orchestrator cannot discover, manage, or prune module resources.

**Recommended standard** items are the blessed convention. Modules that use the
shared deploy/GC framework get them automatically. Modules with their own
tooling are encouraged to adopt them for ecosystem consistency but are not
required to.

## Contract Labels

### `ManagedBy` (`components.platform.opendatahub.io/managed-by`)

The primary orchestrator discovery mechanism. The orchestrator watches all
resources carrying this label. When a module is removed from the DSC, the
orchestrator uses this label to find and prune the module's bootstrap resources.

**Impact of incorrect value:** The orchestrator cannot find the module's
resources. Removal will leave orphaned resources in the cluster.

## Contract Annotations

### `ManagementStateAnnotation` (`component.opendatahub.io/management-state`)

Written by the orchestrator on module CRs to relay the DSC management state
(`Managed` or `Removed`). Module controllers must read this annotation to
determine whether they should reconcile or tear down.

### `ManagedByODHOperator` (`opendatahub.io/managed`)

Resource opt-out convention. When set to `"false"` on a resource, both the
orchestrator and module controllers should treat the resource as create-only:
apply it on initial creation but never update it afterward. This allows cluster
administrators to customize resources (e.g. ConfigMaps, Secrets) without having
the controller overwrite their changes on the next reconciliation.

**Semantics:**
- Absent or `"true"` → resource is fully managed (default behavior).
- `"false"` → create-only; skip updates after the initial apply.

## Recommended Standard Labels

### `PlatformPartOf` (`platform.opendatahub.io/part-of`)

Identifies which controller owns a deployed resource. The reconciler builder
uses this for watch filtering, and the GC action uses it as the label selector.
The standard value is the lowercase Kind name of the controller CR.

**Important:** Both cache selectors and deploy actions must normalize values
identically via `NormalizePartOfValue()`. If they diverge, GC label selection
will not match and resources will be orphaned or incorrectly collected.

### `PlatformDependency` (`platform.opendatahub.io/dependency`)

Marks dependency relationships between platform resources.

### `InfrastructurePartOf` (`infrastructure.opendatahub.io/part-of`)

Same semantics as `PlatformPartOf` but scoped to infrastructure-layer
(CloudManager) resources.

### `Platform` (value: `"platform"`)

The standard value used with `PlatformPartOf` for platform-level resources
such as CRDs.

## Recommended Standard Annotations (Deploy/GC Lifecycle)

These annotations form the protocol between deploy and GC actions. Deploy
stamps them on resources at apply time; GC reads them to determine whether a
resource is stale and should be removed.

### `PlatformVersion` (`platform.opendatahub.io/version`)

Records the release version at deploy time. GC compares the current release
version against this value to detect resources left over from a previous
release.

### `PlatformType` (`platform.opendatahub.io/type`)

Records the platform type at deploy time. GC checks for platform-type changes
(e.g. switching from self-managed to managed).

### `InstanceGeneration` (`platform.opendatahub.io/instance.generation`)

Records the controller CR's `.metadata.generation` at deploy time. GC compares
this to the current generation to detect resources from a previous spec update.

### `InstanceName` (`platform.opendatahub.io/instance.name`)

Records the controller CR's name. The reconciler builder's default `Watches()`
handler maps events on deployed resources back to the owning CR using this
annotation.

### `InstanceUID` (`platform.opendatahub.io/instance.uid`)

Records the controller CR's UID. GC compares this to the current UID to detect
resources from a deleted-and-recreated CR.

## Label Prefixes

The package exports three prefix constants (`ODHAppPrefix`,
`ODHPlatformPrefix`, `ODHInfrastructurePrefix`) for constructing custom label
keys within the ODH namespace.

## NormalizePartOfValue

`NormalizePartOfValue(v string) (string, error)` lowercases and trims
whitespace. It also validates the result against Kubernetes label-value
constraints (alphanumeric with `-`, `_`, `.` allowed; max 63 characters;
must start and end with an alphanumeric character). An empty string is valid.

Both the deploy action (when stamping `PlatformPartOf`) and the GC action
(when building its label selector) must use this function to ensure values
match.
