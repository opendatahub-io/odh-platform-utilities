# ODH Module Operators — Metrics Proposal

**Author:** Heorhii Churhuliia  
**Related ticket:** RHOAIENG-58505  
**Related PR:** [odh-platform-utilities#32](https://github.com/opendatahub-io/odh-platform-utilities/pull/32)  
**Status:** Draft — open for async discussion

---

## Context

PR #32 established a baseline `pkg/metrics` package with two custom metrics:
- `module_reconcile_total`
- `module_reconcile_duration_seconds`

During review, `@lburgazzoli` pointed out that `controller-runtime` already provides
equivalent metrics out of the box. This document analyses what is already available,
identifies the gaps, and proposes a revised set of metrics worth adding to the
shared utilities library.

---

## What controller-runtime already provides (no code needed)

Every module operator built on `controller-runtime` automatically exposes these
metrics on `/metrics` without any extra code:

| Metric | Type | What it tells us |
|---|---|---|
| `controller_runtime_reconcile_total` | Counter | Total reconcile count per controller, labeled by `result` (success/error/requeue) |
| `controller_runtime_reconcile_errors_total` | Counter | Error count per controller |
| `controller_runtime_reconcile_time_seconds` | Histogram | Reconcile loop duration per controller |
| `controller_runtime_active_workers` | Gauge | Currently running reconcile goroutines |
| `controller_runtime_max_concurrent_reconciles` | Gauge | Concurrency limit |
| `workqueue_depth` | Gauge | Items waiting to be reconciled |
| `workqueue_queue_duration_seconds` | Histogram | Time items spend waiting in the queue |

**Conclusion:** The two metrics in PR #32 (`module_reconcile_total` and
`module_reconcile_duration_seconds`) duplicate what `controller-runtime` already
provides. They should be **removed or replaced** with thin wrappers/helpers that
reference the built-in metrics instead.

---

## Gaps — what controller-runtime does NOT provide

The following are not covered by the built-in metrics and would be genuinely
useful for monitoring the modular architecture.

### 1. Module health / condition status

**Problem:** There is no Prometheus metric that exposes the `Ready`, `Degraded`,
or `ProvisioningSucceeded` condition of a module CR. An operator could be running
but stuck in a degraded state — invisible to Prometheus today.

**Proposed metric:**

```
module_condition_status{module="monitoring", condition="Ready", status="True"} 1
module_condition_status{module="monitoring", condition="Degraded", status="False"} 1
```

- **Type:** Gauge (1 = condition matches, 0 = does not match)
- **Labels:** `module`, `condition`, `status`
- **Where to add:** `pkg/metrics/` — helper that module operators call after
  updating conditions

---

### 2. Module management state

**Problem:** There is no metric showing whether a module is in `Managed` or
`Removed` state. Useful for dashboards showing the platform configuration at a
glance.

**Proposed metric:**

```
module_management_state{module="monitoring", state="Managed"} 1
```

- **Type:** Gauge (1 = active state, 0 = inactive)
- **Labels:** `module`, `state`
- **Where to add:** `pkg/metrics/` — set during each reconcile

---

### 3. Precondition / dependency failures

**Problem:** Module operators check for prerequisite operators (e.g. Cluster
Observability Operator, Cert Manager) before deploying. When a prerequisite is
missing the module cannot progress — but today there is no metric for this.

**Proposed metric:**

```
module_precondition_failures_total{module="monitoring", prerequisite="cluster-observability-operator"} 3
```

- **Type:** Counter
- **Labels:** `module`, `prerequisite`
- **Where to add:** `pkg/metrics/` — increment when a precondition check fails

---

### 4. Managed resource count

**Problem:** `pkg/deploy` already exposes `deploy_resources_total` (resources
applied per controller) but it is a cumulative counter, not a gauge. There is no
metric showing how many resources a module currently owns.

**Proposed metric:**

```
module_owned_resources{module="monitoring", kind="Deployment"} 3
module_owned_resources{module="monitoring", kind="ConfigMap"} 12
```

- **Type:** Gauge
- **Labels:** `module`, `kind`
- **Where to add:** `pkg/deploy/` or `pkg/metrics/` — updated after each
  successful deploy

---

### 5. Garbage collection activity

**Problem:** `pkg/controller/gc` deletes stale resources but exposes no metrics.
A sudden spike in GC deletions could indicate a configuration error or a template
rendering bug.

**Proposed metric:**

```
module_gc_deleted_resources_total{module="monitoring", kind="ConfigMap"} 2
```

- **Type:** Counter
- **Labels:** `module`, `kind`
- **Where to add:** `pkg/controller/gc/` — increment on each deletion

---

### 6. Module version / build info

**Problem:** When multiple module versions are deployed (e.g. during a rollout)
there is no metric linking the running pod to its version. This makes correlating
performance changes with releases difficult.

**Proposed metric:**

```
module_build_info{module="monitoring", version="v0.3.1", repo="odh-observability"} 1
```

- **Type:** Gauge (always 1 — info metric pattern)
- **Labels:** `module`, `version`, `repo`
- **Where to add:** `pkg/metrics/` — set once at startup from env vars

---


## Recommended action for PR #32

1. **Remove** `module_reconcile_total` and `module_reconcile_duration_seconds`
   — these duplicate `controller_runtime_reconcile_total` and
   `controller_runtime_reconcile_time_seconds`.

2. **Keep** `ReconcileTimer` helper but rewrite it to use the built-in
   `controller_runtime_*` metrics, or remove it entirely since
   `controller-runtime` instruments the reconcile loop automatically.

3. **Add** `module_condition_status` and `module_precondition_failures_total`
   as the highest-value new metrics (these have no controller-runtime equivalent).

4. Treat the remaining proposals as a follow-up issue to avoid scope creep in PR #32.

---

## Open questions for the team

1. Should `ReconcileTimer` be kept as a convenience wrapper over built-in metrics,
   or removed entirely?
2. Should condition metrics be pushed from module code or pulled by a shared
   status-watching controller?
3. Is `module_owned_resources` worth the complexity of maintaining a per-kind gauge?
4. Which metrics should block PR #32 vs be tracked as follow-up issues?
