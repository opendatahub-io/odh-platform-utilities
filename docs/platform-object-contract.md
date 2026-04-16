# PlatformObject Contract

This document describes the contract between the ODH Operator (orchestrator)
and module controllers, as defined by the types in `api/common`.

## Overview

Every module CRD that participates in the ODH platform must implement the
`PlatformObject` interface:

```go
type PlatformObject interface {
    client.Object       // Kubernetes metadata + runtime.Object
    WithStatus          // GetStatus() *Status
    ConditionsAccessor  // GetConditions() / SetConditions()
    WithReleases        // GetReleaseStatus() / SetReleaseStatus()
}
```

The orchestrator reads module CR status generically through this interface to:
- Aggregate health across modules
- Detect module versions for upgrade mode
- Gate DAG progression during upgrades

## Types

### Status

The common status block embedded in every module CR:

```go
type Status struct {
    Phase              Phase       `json:"phase,omitempty"`
    ObservedGeneration int64       `json:"observedGeneration,omitempty"`
    Conditions         []Condition `json:"conditions,omitempty"`
}
```

| Field | Purpose |
|-------|---------|
| `Phase` | Top-level lifecycle phase (`Ready` or `Not Ready`) |
| `ObservedGeneration` | Latest `.metadata.generation` the controller processed |
| `Conditions` | Detailed condition observations (map-keyed by `type`) |

### Condition

An individual observation of module state:

```go
type Condition struct {
    Type               string                    `json:"type"`
    Status             metav1.ConditionStatus    `json:"status"`
    Reason             string                    `json:"reason"`
    Message            string                    `json:"message,omitempty"`
    Severity           ConditionSeverity         `json:"severity,omitempty"`
    LastTransitionTime metav1.Time               `json:"lastTransitionTime"`
    ObservedGeneration int64                     `json:"observedGeneration,omitempty"`
}
```

### ComponentRelease / ComponentReleaseStatus

Release tracking for upgrade mode:

```go
type ComponentRelease struct {
    Name    string `json:"name"`
    Version string `json:"version"`
    RepoURL string `json:"repoUrl,omitempty"`
}

type ComponentReleaseStatus struct {
    Releases []ComponentRelease `json:"releases,omitempty"`
}
```

### ManagementSpec

User intent for component lifecycle:

```go
type ManagementSpec struct {
    ManagementState ManagementState `json:"managementState,omitempty"`
}
```

Values: `Managed` (default) or `Removed`.

> **Note:** This type is decoupled from `github.com/openshift/api`. It uses
> a plain `string` type to avoid pulling in the OpenShift API dependency tree
> for vanilla Kubernetes consumers.

## Mandatory Condition Types

The orchestrator evaluates these by **exact string match**. All module
controllers must set these conditions.

### `Ready` (`ConditionTypeReady`)

Top-level aggregate condition. The orchestrator checks this before advancing
to the next runlevel in the DAG.

| Status | Meaning |
|--------|---------|
| `True` | Module is fully operational |
| `False` | Module is not available |
| `Unknown` | Module health cannot be determined |

### `ProvisioningSucceeded` (`ConditionTypeProvisioningSucceeded`)

Reflects the result of manifest application. The orchestrator reads this for
status aggregation.

| Status | Meaning |
|--------|---------|
| `True` | All manifests applied successfully |
| `False` | Manifest application failed |

### `Degraded` (`ConditionTypeDegraded`)

Indicates the module is functioning but impaired. The orchestrator reports
this in cluster-wide health.

| Status | Meaning |
|--------|---------|
| `True` | Module is degraded (partial availability) |
| `False` | Module is not degraded |

### Condition Combinations

| Ready | Degraded | Meaning |
|-------|----------|---------|
| `True` | `False` | Fully healthy |
| `True` | `True` | Partially available — some features impaired |
| `False` | `True` | Not available, with known degradation cause |
| `False` | `False` | Not available (e.g. still provisioning) |

## How the Orchestrator Reads These

The orchestrator evaluates module CRs in the following order:

1. **Phase** (`.status.phase`): Quick check — if not `Ready`, the module is
   considered unavailable.
2. **Ready condition**: Primary gate for DAG progression. The orchestrator
   will not advance to the next runlevel until all modules in the current
   runlevel have `Ready=True`.
3. **ProvisioningSucceeded condition**: Used for status aggregation and
   reporting. Not a gate for DAG progression by itself.
4. **Degraded condition**: Reported in cluster-wide health. Does not block
   DAG progression if `Ready=True`.
5. **Releases** (`.status.releases`): Read to detect module versions and
   determine whether upgrade mode should be entered.
6. **ObservedGeneration**: Compared with `.metadata.generation` to detect
   whether the controller has processed the latest spec. Stale conditions
   (where `observedGeneration < generation`) are treated as `Unknown`.

### Fields That Are Evaluated vs. Informational

| Field | Evaluated | Informational |
|-------|-----------|---------------|
| `.status.phase` | ✓ | |
| `Ready` condition | ✓ (DAG gate) | |
| `ProvisioningSucceeded` condition | ✓ (aggregation) | |
| `Degraded` condition | ✓ (health) | |
| `.status.releases` | ✓ (upgrade mode) | |
| `.status.observedGeneration` | ✓ (staleness) | |
| `Condition.Message` | | ✓ |
| `Condition.Severity` | | ✓ |

## Phase Constants

```go
const (
    PhaseReady    Phase = "Ready"      // Module is fully operational
    PhaseNotReady Phase = "Not Ready"  // Module is not yet available
)
```

## Example: Minimal PlatformObject Implementation

This is a complete, copy-pasteable starting point for a new module CRD:

```go
package v1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "github.com/opendatahub-io/odh-platform-utilities/api/common"
)

// MyComponentSpec defines the desired state.
type MyComponentSpec struct {
    common.ManagementSpec `json:",inline"`
}

// MyComponentStatus defines the observed state.
type MyComponentStatus struct {
    common.Status                 `json:",inline"`
    common.ComponentReleaseStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-mycomponent'",message="MyComponent name must be default-mycomponent"
type MyComponent struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              MyComponentSpec   `json:"spec,omitempty"`
    Status            MyComponentStatus `json:"status,omitempty"`
}

// --- PlatformObject implementation ---

func (m *MyComponent) GetStatus() *common.Status {
    return &m.Status.Status
}

func (m *MyComponent) GetConditions() []common.Condition {
    return m.Status.Conditions
}

func (m *MyComponent) SetConditions(conditions []common.Condition) {
    m.Status.Conditions = conditions
}

func (m *MyComponent) GetReleaseStatus() *common.ComponentReleaseStatus {
    return &m.Status.ComponentReleaseStatus
}

func (m *MyComponent) SetReleaseStatus(status common.ComponentReleaseStatus) {
    m.Status.ComponentReleaseStatus = status
}

// Compile-time check.
var _ common.PlatformObject = (*MyComponent)(nil)
```

## What NOT to Do

### Omitting `observedGeneration`

**Wrong:** Setting conditions without `ObservedGeneration`.

```go
// BAD: no observedGeneration — orchestrator may treat as stale
condition := common.Condition{
    Type:   common.ConditionTypeReady,
    Status: metav1.ConditionTrue,
    Reason: "Ready",
}
```

**Right:** Always set `ObservedGeneration` from the CR's `.metadata.generation`.

```go
condition := common.Condition{
    Type:               common.ConditionTypeReady,
    Status:             metav1.ConditionTrue,
    Reason:             "Ready",
    ObservedGeneration: myComponent.Generation,
    LastTransitionTime: metav1.Now(),
}
```

### Mixing Up Condition Semantics

**Wrong:** Setting `Ready=True` when `ProvisioningSucceeded=False`.

The `Ready` condition should be a true aggregate. If provisioning has not
succeeded, `Ready` should be `False`. Set `Ready=True` only when all
sub-conditions indicate the module is genuinely available.

### Inventing Custom Phase Values

**Wrong:** Using `Phase: "Initializing"` or `Phase: "Error"`.

The orchestrator only recognizes `Ready` and `Not Ready`. Use conditions for
granular state. The phase is a coarse-grained signal.

### Forgetting `WithReleases`

If your module CR does not implement `WithReleases`, the orchestrator cannot
detect your module's version. This breaks upgrade mode. Even if you have no
sub-components, return an empty `ComponentReleaseStatus`.

## Singleton Enforcement

The Onboarding Guide (Section 2.1) mandates that all module CRDs are
cluster-scoped singletons with enforced naming.

### CEL Validation Rule (Preferred)

Use a `+kubebuilder:validation:XValidation` marker on the CRD type to enforce
the singleton name at the schema level:

```go
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-mycomponent'",message="MyComponent name must be default-mycomponent"
type MyComponent struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    // ...
}
```

**Advantages:**
- Enforced by the API server with zero webhook infrastructure.
- Declarative — the constraint is visible in the CRD schema.
- Cannot be bypassed by direct etcd writes (unlike webhooks).

**When to use:** This is the default recommendation for all new module CRDs.

### Validating Webhook (Fallback)

Use a validating webhook when CEL cannot express the constraint, for example
when enforcing cross-version singleton constraints (preventing creation of a
v1beta1 resource when a v1 resource already exists).

```go
package webhook

import (
    "context"

    "k8s.io/apimachinery/pkg/runtime/schema"
    webhookutil "github.com/opendatahub-io/odh-platform-utilities/pkg/webhook"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var myGVK = schema.GroupVersionKind{
    Group:   "components.opendatahub.io",
    Version: "v1",
    Kind:    "MyComponent",
}

type MyComponentWebhook struct {
    reader  client.Reader
}

func (w *MyComponentWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
    return webhookutil.ValidateSingletonCreation(ctx, w.reader, &req, myGVK)
}
```

**When to use:** Only when CEL rules are insufficient (cross-version
scenarios, complex multi-resource constraints).

### Combining CEL + Webhook

For maximum safety, use both:
- CEL enforces the naming convention (`self.metadata.name == 'default-mycomponent'`).
- Webhook enforces the singleton count across API versions.
