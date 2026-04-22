// Package common defines the contract types, interfaces, and constants shared
// between the ODH Operator (orchestrator) and module controllers.
//
// Every module CRD that participates in the ODH platform must implement the
// [PlatformObject] interface. The orchestrator reads module CR status
// generically through these types to aggregate health, detect versions, and
// gate DAG progression during upgrades.
//
// # Types
//
// The core status types are [Status], [Condition], [ComponentRelease], and
// [ComponentReleaseStatus]. Together they describe a module's health, the
// conditions that led to that health assessment, and the software versions
// the module is running.
//
// [ManagementSpec] carries the user's intent about whether a module should be
// actively managed or removed.
//
// # Interfaces
//
// Three accessor interfaces abstract status field access so the orchestrator
// does not need concrete knowledge of each module's status struct:
//
//   - [WithStatus] — access to the common [Status] block
//   - [ConditionsAccessor] — read/write access to the conditions slice
//   - [WithReleases] — access to [ComponentReleaseStatus] for version tracking
//
// [PlatformObject] composes [client.Object] with all three accessors.
//
// # Condition Types
//
// The orchestrator evaluates three mandatory condition types by exact string
// match: [ConditionTypeReady], [ConditionTypeProvisioningSucceeded], and
// [ConditionTypeDegraded]. Module controllers must set these conditions to
// participate in cluster-wide health aggregation.
//
// # Phases
//
// [PhaseReady] and [PhaseNotReady] are the two phase values the orchestrator
// checks on .status.phase to determine top-level module health.
package common
