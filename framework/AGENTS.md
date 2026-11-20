# Framework Contributor Guide

The framework module is an opinionated controller framework. Preserve its
reconciliation, status, finalizer, and action-pipeline contracts when changing
framework code.

## Read the relevant reference first

| Change area | Required reference |
| --- | --- |
| `controller/reconciler/**` or `controller/actions/errors/**` | [Action error semantics](./docs/action-error-semantics.md) |
| `controller/conditions/**` or status updates | [Status and condition management](../docs/module-operator-scenarios.md#5-status-and-condition-management) |
| Render actions | [Manifest rendering](../docs/module-operator-scenarios.md#2-manifest-rendering) |
| Deploy or ownership actions | [Resource deployment and ownership](../docs/module-operator-scenarios.md#3-resource-deployment-and-ownership) |
| GC actions | [Garbage collection](../docs/module-operator-scenarios.md#4-garbage-collection) |
| Builder or action-pipeline changes | [Pipeline approach](../docs/module-operator-scenarios.md#option-a-pipeline-approach-reconcilerbuilder) |
| Public API changes | [Versioning](../docs/VERSIONING.md) |

For action-error or reconciler changes, keep the action-error reference, GoDoc,
and behavior tests aligned. The focused framework reference takes precedence
over high-level scenario examples when they differ.

## Validation

Run framework checks through its Makefile (for example,
`make -C framework test` and `make -C framework verify-fmt`). Keep tests in
the external `_test` package, use `t.Parallel()`, and cover apply and deletion
paths when changing shared action-outcome behavior.
