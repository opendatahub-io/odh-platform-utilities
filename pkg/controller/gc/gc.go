package gc

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	odhAnnotations "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	odhLabels "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

func gvkCustomResourceDefinition() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	}
}

func gvkLease() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "coordination.k8s.io",
		Version: "v1",
		Kind:    "Lease",
	}
}

var errInvalidRunParams = errors.New("RunParams requires non-nil Client, DynamicClient, DiscoveryClient, and Owner")

// RunParams holds the per-reconcile-cycle inputs for garbage collection.
// These values change on each reconcile invocation, as opposed to the
// Collector configuration which is set once at construction time.
type RunParams struct {
	Client          client.Client
	DynamicClient   dynamic.Interface
	DiscoveryClient discovery.DiscoveryInterface
	Owner           client.Object
	Version         string
	PlatformType    string
}

// Option configures a Collector.
type Option func(*Collector)

// Collector handles garbage collection of stale Kubernetes resources. It
// compares what is currently deployed on the cluster against what the
// controller just rendered, and deletes the difference.
//
// Create a Collector once with New and call Run on each reconcile cycle.
type Collector struct {
	objectPredicateFn ObjectPredicateFn
	typePredicateFn   TypePredicateFn
	namespaceFn       func(context.Context) (string, error)
	labels            map[string]string
	selector          labels.Selector
	unremovables      map[schema.GroupVersionKind]struct{}
	propagationPolicy client.PropagationPolicy
	onlyOwned         bool
	metricsEnabled    bool
}

// New creates a new Collector with sensible defaults:
//   - Default object predicate checks deploy/GC annotation protocol
//   - Default type predicate allows all types
//   - Only owned resources are collected (onlyOwned = true)
//   - Foreground deletion propagation
//   - CRDs and Leases are unremovable
//   - No namespace configured (must be set via InNamespace or InNamespaceFn)
func New(opts ...Option) *Collector {
	c := &Collector{
		objectPredicateFn: DefaultObjectPredicate,
		typePredicateFn:   DefaultTypePredicate,
		onlyOwned:         true,
		propagationPolicy: client.PropagationPolicy(metav1.DeletePropagationForeground),
		unremovables: map[schema.GroupVersionKind]struct{}{
			gvkCustomResourceDefinition(): {},
			gvkLease():                    {},
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	if len(c.labels) > 0 {
		c.selector = labels.SelectorFromSet(c.labels)
	}

	return c
}

// Run executes one garbage collection cycle. It discovers available API
// resources, filters to those the controller has RBAC permission to delete,
// lists matching resources by label selector, applies predicates, and deletes
// stale resources.
//
// Callers should skip calling Run when nothing was generated in the current
// reconcile cycle to avoid expensive API discovery on no-op reconciles.
func (c *Collector) Run(ctx context.Context, params RunParams) error {
	if params.Client == nil || params.DynamicClient == nil || params.DiscoveryClient == nil || params.Owner == nil {
		return errInvalidRunParams
	}

	l := logf.FromContext(ctx)

	ownerGVK, err := resources.GetGroupVersionKindForObject(
		params.Client.Scheme(), params.Owner,
	)
	if err != nil {
		return fmt.Errorf("unable to resolve owner GVK: %w", err)
	}

	controllerName := strings.ToLower(ownerGVK.Kind)

	if c.metricsEnabled {
		CyclesTotal.WithLabelValues(controllerName).Inc()
	}

	items, err := c.computeDeletableTypes(ctx, params)
	if err != nil {
		return fmt.Errorf("unable to refresh collectable resources: %w", err)
	}

	lo := metav1.ListOptions{
		LabelSelector: c.getOrComputeSelector(controllerName).String(),
	}

	l.V(3).Info("run", "selector", lo.LabelSelector)

	for _, res := range items {
		collectErr := c.collectType(ctx, params, ownerGVK, controllerName, res, lo)
		if collectErr != nil {
			return collectErr
		}
	}

	return nil
}

func (c *Collector) collectType(
	ctx context.Context,
	params RunParams,
	ownerGVK schema.GroupVersionKind,
	controllerName string,
	res resources.Resource,
	lo metav1.ListOptions,
) error {
	canBeDeleted, err := c.isTypeDeletable(params, res.GroupVersionKind())
	if err != nil {
		return fmt.Errorf("cannot determine if resource %s can be deleted: %w", res.String(), err)
	}

	if !canBeDeleted {
		return nil
	}

	listed, err := c.listResources(ctx, params.DynamicClient, res, lo)
	if err != nil {
		return fmt.Errorf("cannot list child resources %s: %w", res.String(), err)
	}

	deleted, err := c.deleteResources(ctx, params, ownerGVK, listed)
	if deleted > 0 && c.metricsEnabled {
		DeletedTotal.WithLabelValues(controllerName).Add(float64(deleted))
	}

	if err != nil {
		return fmt.Errorf("error processing items to delete: %w", err)
	}

	return nil
}

func (c *Collector) computeDeletableTypes(
	ctx context.Context,
	params RunParams,
) ([]resources.Resource, error) {
	res, err := resources.ListAvailableAPIResources(params.DiscoveryClient)
	if err != nil {
		return nil, fmt.Errorf("failure discovering resources: %w", err)
	}

	ns, err := c.resolveNamespace(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to compute namespace: %w", err)
	}

	items, err := ListAuthorizedResources(ctx, params.Client, res, ns, []string{VerbDelete})
	if err != nil {
		return nil, fmt.Errorf("failure listing authorized deletable resources: %w", err)
	}

	return items, nil
}

func (c *Collector) resolveNamespace(ctx context.Context) (string, error) {
	if c.namespaceFn == nil {
		return "", nil
	}

	return c.namespaceFn(ctx)
}

func (c *Collector) listResources(
	ctx context.Context,
	dc dynamic.Interface,
	res resources.Resource,
	opts metav1.ListOptions,
) ([]unstructured.Unstructured, error) {
	items, err := dc.Resource(res.GroupVersionResource()).Namespace("").List(ctx, opts)

	switch {
	case k8serr.IsForbidden(err) || k8serr.IsMethodNotSupported(err) || k8serr.IsNotFound(err):
		logf.FromContext(ctx).V(3).Info(
			"cannot list resource",
			"reason", err.Error(),
			"gvk", res.GroupVersionKind(),
		)

		return nil, nil
	case err != nil:
		return nil, err
	default:
		return items.Items, nil
	}
}

func (c *Collector) isTypeDeletable(
	params RunParams,
	gvk schema.GroupVersionKind,
) (bool, error) {
	if c.isUnremovable(gvk) {
		return false, nil
	}

	return c.typePredicateFn(params, gvk)
}

func (c *Collector) isObjectDeletable(
	params RunParams,
	ownerGVK schema.GroupVersionKind,
	obj unstructured.Unstructured,
) (bool, error) {
	if c.isUnremovable(obj.GroupVersionKind()) {
		return false, nil
	}

	if resources.HasAnnotationWithValue(&obj, odhAnnotations.ManagedByODHOperator, "false") {
		return false, nil
	}

	if c.onlyOwned {
		o, err := resources.IsOwnedByType(&obj, ownerGVK)
		if err != nil {
			return false, err
		}

		if !o {
			return false, nil
		}
	}

	return c.objectPredicateFn(params, obj)
}

func (c *Collector) deleteResources(
	ctx context.Context,
	params RunParams,
	ownerGVK schema.GroupVersionKind,
	items []unstructured.Unstructured,
) (int, error) {
	deleted := 0

	for i := range items {
		canBeDeleted, err := c.isObjectDeletable(params, ownerGVK, items[i])
		if err != nil {
			return deleted, fmt.Errorf(
				"cannot determine if object %s in namespace %q can be deleted: %w",
				items[i].GetName(),
				items[i].GetNamespace(),
				err,
			)
		}

		if !canBeDeleted {
			continue
		}

		if !items[i].GetDeletionTimestamp().IsZero() {
			continue
		}

		delErr := c.delete(ctx, params.Client, items[i])
		if delErr != nil {
			return deleted, delErr
		}

		deleted++
	}

	return deleted, nil
}

func (c *Collector) delete(
	ctx context.Context,
	cli client.Client,
	resource unstructured.Unstructured,
) error {
	logf.FromContext(ctx).Info(
		"delete",
		"gvk", resource.GroupVersionKind(),
		"ns", resource.GetNamespace(),
		"name", resource.GetName(),
	)

	err := cli.Delete(ctx, &resource, c.propagationPolicy)
	if err != nil && !k8serr.IsNotFound(err) {
		return fmt.Errorf(
			"cannot delete resources gvk: %s, namespace: %s, name: %s, reason: %w",
			resource.GroupVersionKind().String(),
			resource.GetNamespace(),
			resource.GetName(),
			err,
		)
	}

	return nil
}

func (c *Collector) getOrComputeSelector(partOf string) labels.Selector {
	if c.selector != nil {
		return c.selector
	}

	return labels.SelectorFromSet(map[string]string{
		odhLabels.PlatformPartOf: partOf,
	})
}

func (c *Collector) isUnremovable(gvk schema.GroupVersionKind) bool {
	_, ok := c.unremovables[gvk]
	return ok
}

// --- Options ---

// WithLabel adds a label requirement to the resource selector.
func WithLabel(name string, value string) Option {
	return func(c *Collector) {
		if c.labels == nil {
			c.labels = map[string]string{}
		}

		c.labels[name] = value
	}
}

// WithLabels adds multiple label requirements to the resource selector.
func WithLabels(values map[string]string) Option {
	return func(c *Collector) {
		if c.labels == nil {
			c.labels = map[string]string{}
		}

		maps.Copy(c.labels, values)
	}
}

// WithUnremovables adds GVKs that should never be deleted by GC.
func WithUnremovables(items ...schema.GroupVersionKind) Option {
	return func(c *Collector) {
		for _, item := range items {
			c.unremovables[item] = struct{}{}
		}
	}
}

// WithObjectPredicate sets a custom predicate to decide whether a specific
// object should be deleted. The default predicate uses the deploy/GC
// annotation protocol.
func WithObjectPredicate(fn ObjectPredicateFn) Option {
	return func(c *Collector) {
		if fn == nil {
			return
		}

		c.objectPredicateFn = fn
	}
}

// WithTypePredicate sets a custom predicate to decide whether a resource
// type (GVK) should be considered for GC at all.
func WithTypePredicate(fn TypePredicateFn) Option {
	return func(c *Collector) {
		if fn == nil {
			return
		}

		c.typePredicateFn = fn
	}
}

// WithOnlyCollectOwned controls whether GC only deletes resources that have
// an owner reference matching the controller CR's GVK. Default is true.
func WithOnlyCollectOwned(value bool) Option {
	return func(c *Collector) {
		c.onlyOwned = value
	}
}

// InNamespace sets a static namespace for RBAC permission checks.
func InNamespace(ns string) Option {
	return func(c *Collector) {
		c.namespaceFn = func(_ context.Context) (string, error) {
			return ns, nil
		}
	}
}

// InNamespaceFn sets a dynamic namespace resolver for RBAC permission checks.
func InNamespaceFn(fn func(context.Context) (string, error)) Option {
	return func(c *Collector) {
		if fn == nil {
			return
		}

		c.namespaceFn = fn
	}
}

// WithMetrics enables Prometheus metrics recording for this Collector.
// Callers must also invoke RegisterMetrics once at startup to register
// the metric descriptors with the controller-runtime registry.
func WithMetrics() Option {
	return func(c *Collector) {
		c.metricsEnabled = true
	}
}

// WithDeletePropagationPolicy sets the deletion propagation policy.
// Default is metav1.DeletePropagationForeground.
func WithDeletePropagationPolicy(policy metav1.DeletionPropagation) Option {
	return func(c *Collector) {
		c.propagationPolicy = client.PropagationPolicy(policy)
	}
}
