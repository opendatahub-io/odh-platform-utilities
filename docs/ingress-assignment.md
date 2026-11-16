# Ingress assignment resolver

`pkg/ingressassignment.Resolve` reads a Namespace's ingress assignment and
returns the selected configured `Ingress` unchanged, including its ingress name,
Gateway name and namespace, optional hostname, and `IsDefault`. Pass the manager's
cached `client.Client`, the Namespace name, and the configured ingresses.
Ingress names and Gateway names must each be unique across the list, even when
Gateway namespaces differ.
Mark at most one ingress as the default. Include unready ingresses so they
remain selectable. A hostname is optional.

```go
ingresses := []ingressassignment.Ingress{
    {
        Name: "default", GatewayName: "data-science-gateway",
        GatewayNamespace: "openshift-ingress",
        IsDefault: true,
    },
    {
        Name: "dev", GatewayName: "data-science-gateway-dev",
        GatewayNamespace: "openshift-ingress",
        Hostname: "dev.example.com",
    },
}
selected, err := ingressassignment.Resolve(ctx, k8sClient, namespaceName, ingresses)
if err != nil {
    return err
}
```

The Namespace annotation `opendatahub.io/ingress-name` matches `Name` exactly.
The resolver returns `Name`, `GatewayName`, and `GatewayNamespace`; consumers
remain responsible for constructing the HTTPRoute Gateway `parentRef`.

`selected.Hostname` is the hostname supplied in the selected ingress. An empty
hostname is valid. If supplied and needed by the component, it can be used for
the HTTPRoute's `spec.hostnames` or a displayed URL. The resolver builds no
HTTPRoute and reads no Gateway resources; it does not check whether a configured
Gateway exists or is ready. Call `Resolve` once per reconciliation.

An absent `opendatahub.io/ingress-name` selects the default, if configured;
otherwise it returns `ErrUnknownIngress`. Explicitly naming the default ingress
selects the same ingress. An exact match with another ingress name selects that
configured ingress. Unknown, removed, or empty annotated names return an empty
`Ingress` and an error identifying the Namespace and annotated name, without
default fallback. `Resolve` only reads the current Namespace.

## Errors

Every error returns an empty `Ingress`. Use the wrapped errors to distinguish
failure types:

- `errors.Is(err, ingressassignment.ErrUnknownIngress)` identifies unknown,
  removed, or empty annotated names, or an absent annotation without a default.
- `errors.Is(err, ingressassignment.ErrInvalidIngresses)` identifies invalid
  ingress lists: multiple defaults, empty ingress or Gateway names or Gateway
  namespaces, or duplicate ingress or Gateway names. Validation runs before
  the Namespace read.
- Namespace read failures wrap the underlying client error with the Namespace
  name. Kubernetes error checks still work, including
  `apierrors.IsNotFound(err)` for a missing Namespace
  (`apierrors` is `k8s.io/apimachinery/pkg/api/errors`).

## Permissions and watches

A direct API client needs `get` on core `namespaces`, which are cluster scoped.
A controller-runtime manager client using its cache needs `list` and `watch`
for the Namespace informer. Grant all three through a ClusterRole when using
the resolver in a controller:

```yaml
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["get", "list", "watch"]
```

Watch `Namespace` create and delete events and updates to its
`opendatahub.io/ingress-name` annotation. Map each event to the component
objects affected in that Namespace so they reconcile again; the Namespace
name alone is not necessarily a reconcile request for those objects. Handle
annotation removal as a change back to the default, or to `ErrUnknownIngress`
when no default is configured. If using
`pkg/controller/predicates.AnnotationChangedPredicate`, add delete handling
separately because that predicate rejects delete events. Also requeue affected
objects when the configured ingress list, Gateway references, or hostnames change.

The resolver does not read or derive ingress readiness or generation. Consumers
that report readiness should use the authoritative config status directly,
including its current-generation conditions and any required reason precedence.
