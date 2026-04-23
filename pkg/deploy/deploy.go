package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

// Mode selects the mechanism used to apply resources.
type Mode string

const (
	// ModePatch applies resources using Server-Side Apply via the Patch endpoint
	// with a Create fallback for new resources.
	ModePatch Mode = "patch"
	// ModeSSA applies resources using Server-Side Apply.
	ModeSSA Mode = "ssa"
)

// ErrUnsupportedMode is returned when an unknown deploy mode is used.
var ErrUnsupportedMode = errors.New("unsupported deploy mode")

// SortFn defines a function that reorders resources before deployment.
type SortFn func(ctx context.Context, resources []unstructured.Unstructured) ([]unstructured.Unstructured, error)

// ReleaseInfo provides release metadata stamped as annotations on every
// deployed resource for GC and diagnostics.
type ReleaseInfo struct {
	Type    string
	Version string
}

// DeployInput contains per-invocation parameters that vary between
// reconciliation loops.
type DeployInput struct {
	Client    client.Client
	Owner     client.Object
	Release   ReleaseInfo
	Resources []unstructured.Unstructured
}

//nolint:gochecknoglobals
var crdGVK = schema.GroupVersionKind{
	Group:   "apiextensions.k8s.io",
	Version: "v1",
	Kind:    "CustomResourceDefinition",
}

// Deployer applies rendered Kubernetes resources to the cluster.
type Deployer struct {
	sortFn          SortFn
	cache           *Cache
	labels          map[string]string
	annotations     map[string]string
	mergeStrategies map[schema.GroupVersionKind]MergeFn
	fieldOwner      string
	crdFieldOwner   string
	managedKey      string
	mode            Mode
}

// Option configures a Deployer.
type Option func(*Deployer)

// WithFieldOwner sets the SSA field owner for non-CRD resources. If empty, the
// lowercased Kind of the Owner is used.
func WithFieldOwner(value string) Option {
	return func(d *Deployer) {
		d.fieldOwner = value
	}
}

// WithCRDFieldOwner sets the SSA field owner used for CRD resources. Defaults
// to "platform.opendatahub.io".
func WithCRDFieldOwner(value string) Option {
	return func(d *Deployer) {
		d.crdFieldOwner = value
	}
}

// WithMode selects the deploy mechanism (SSA or Patch). Default is ModeSSA.
func WithMode(value Mode) Option {
	return func(d *Deployer) {
		d.mode = value
	}
}

// WithLabel adds a single label that will be stamped on every deployed
// resource.
func WithLabel(name, value string) Option {
	return func(d *Deployer) {
		if d.labels == nil {
			d.labels = map[string]string{}
		}

		d.labels[name] = value
	}
}

// WithLabels merges labels that will be stamped on every deployed resource.
func WithLabels(values map[string]string) Option {
	return func(d *Deployer) {
		if d.labels == nil {
			d.labels = map[string]string{}
		}

		maps.Copy(d.labels, values)
	}
}

// WithAnnotation adds a single annotation that will be stamped on every
// deployed resource.
func WithAnnotation(name, value string) Option {
	return func(d *Deployer) {
		if d.annotations == nil {
			d.annotations = map[string]string{}
		}

		d.annotations[name] = value
	}
}

// WithAnnotations merges annotations that will be stamped on every deployed
// resource.
func WithAnnotations(values map[string]string) Option {
	return func(d *Deployer) {
		if d.annotations == nil {
			d.annotations = map[string]string{}
		}

		maps.Copy(d.annotations, values)
	}
}

// WithCache enables deploy caching. Additional CacheOpt values configure the
// cache (e.g. WithTTL).
func WithCache(opts ...CacheOpt) Option {
	return func(d *Deployer) {
		d.cache = NewCache(opts...)
	}
}

// WithSortFn sets a custom sort function to reorder resources before
// deploying.
func WithSortFn(fn SortFn) Option {
	return func(d *Deployer) {
		d.sortFn = fn
	}
}

// WithApplyOrder is a convenience option that sorts resources into dependency
// order (CRDs first, webhooks last) before deploying. It uses
// resources.SortByApplyOrder.
func WithApplyOrder() Option {
	return WithSortFn(resources.SortByApplyOrder)
}

// WithMergeStrategy registers a MergeFn for a specific GVK. When deploying a
// resource of that GVK and a live version already exists, the MergeFn is
// called to merge user-customised fields from the live object into the desired
// manifest before apply.
func WithMergeStrategy(gvk schema.GroupVersionKind, fn MergeFn) Option {
	return func(d *Deployer) {
		if d.mergeStrategies == nil {
			d.mergeStrategies = map[schema.GroupVersionKind]MergeFn{}
		}

		d.mergeStrategies[gvk] = fn
	}
}

// WithManagedAnnotation overrides the annotation key used for the "managed"
// opt-out convention. Default is annotations.ManagedByODHOperator
// ("opendatahub.io/managed").
func WithManagedAnnotation(key string) Option {
	return func(d *Deployer) {
		d.managedKey = key
	}
}

// NewDeployer creates a Deployer with the given options.
func NewDeployer(opts ...Option) *Deployer {
	d := &Deployer{
		mode:          ModeSSA,
		crdFieldOwner: "platform.opendatahub.io",
		managedKey:    annotations.ManagedByODHOperator,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Deploy applies all resources from input to the cluster using the configured
// mode, caching, merge strategies, and annotation stamping.
func (d *Deployer) Deploy(ctx context.Context, input DeployInput) error {
	resourceList, err := d.prepareResources(ctx, input)
	if err != nil {
		return err
	}

	controllerName, err := d.resolveControllerName(input)
	if err != nil {
		return err
	}

	for i := range resourceList {
		ok, err := d.deployOne(ctx, input, resourceList[i], controllerName)
		if err != nil {
			return fmt.Errorf("failure deploying resource %s/%s: %w",
				resourceList[i].GetNamespace(), resourceList[i].GetName(), err)
		}

		if ok && controllerName != "" {
			DeployedResourcesTotal.WithLabelValues(controllerName).Inc()
		}
	}

	return nil
}

func (d *Deployer) prepareResources(
	ctx context.Context,
	input DeployInput,
) ([]unstructured.Unstructured, error) {
	resourceList := input.Resources

	if d.sortFn != nil {
		sorted, err := d.sortFn(ctx, resourceList)
		if err != nil {
			return nil, fmt.Errorf("failed to sort resources: %w", err)
		}

		resourceList = sorted
	}

	if d.cache != nil {
		d.cache.Sync()
	}

	return resourceList, nil
}

func (d *Deployer) resolveControllerName(input DeployInput) (string, error) {
	if d.fieldOwner != "" {
		return d.fieldOwner, nil
	}

	if input.Owner == nil {
		return "", nil
	}

	kind, err := resources.KindForObject(input.Client.Scheme(), input.Owner)
	if err != nil {
		return "", fmt.Errorf("failed to resolve controller name: %w", err)
	}

	return strings.ToLower(kind), nil
}

func (d *Deployer) deployOne(
	ctx context.Context,
	input DeployInput,
	res unstructured.Unstructured,
	controllerName string,
) (bool, error) {
	current := resources.GvkToUnstructured(res.GroupVersionKind())

	lookupErr := input.Client.Get(ctx, client.ObjectKeyFromObject(&res), current)

	switch {
	case k8serr.IsNotFound(lookupErr):
		current = nil
	case lookupErr != nil:
		return false, fmt.Errorf("failed to lookup object %s/%s: %w",
			res.GetNamespace(), res.GetName(), lookupErr)
	default:
		if resources.GetAnnotation(current, d.managedKey) == "false" {
			return false, nil
		}
	}

	if res.GroupVersionKind() == crdGVK {
		return d.deployCRD(ctx, input, res, current)
	}

	return d.deployResource(ctx, input, res, current, controllerName)
}

func (d *Deployer) shouldSkip(current, desired *unstructured.Unstructured) (bool, error) {
	if desired == nil {
		return false, nil
	}

	if current != nil && !current.GetDeletionTimestamp().IsZero() {
		if d.cache != nil {
			err := d.cache.Delete(current, desired)
			if err != nil {
				return false, err
			}
		}

		return true, nil
	}

	if d.cache == nil {
		return false, nil
	}

	return d.cache.Has(current, desired)
}

func (d *Deployer) deployCRD(
	ctx context.Context,
	input DeployInput,
	obj unstructured.Unstructured,
	current *unstructured.Unstructured,
) (bool, error) {
	obj = *obj.DeepCopy()

	resources.SetLabels(&obj, d.labels)
	resources.SetAnnotations(&obj, d.annotations)
	resources.SetLabel(&obj, labels.PlatformPartOf, labels.Platform)

	shouldSkip, err := d.shouldSkip(current, &obj)
	if err != nil {
		return false, err
	}

	if shouldSkip {
		return false, nil
	}

	origObj := obj.DeepCopy()

	fo := d.crdFieldOwner

	deployedObj, err := d.applyWithMode(ctx, input.Client, &obj, current, fo)
	if err != nil {
		return false, client.IgnoreNotFound(err)
	}

	return true, d.cacheResult(deployedObj, origObj)
}

func (d *Deployer) deployResource(
	ctx context.Context,
	input DeployInput,
	obj unstructured.Unstructured,
	current *unstructured.Unstructured,
	fieldOwner string,
) (bool, error) {
	obj = *obj.DeepCopy()

	d.stampMetadata(&obj, input, fieldOwner)

	shouldSkip, err := d.shouldSkip(current, &obj)
	if err != nil {
		return false, err
	}

	if shouldSkip {
		return false, nil
	}

	origObj := obj.DeepCopy()

	deployedObj, err := d.applyResource(ctx, input, &obj, current, fieldOwner)
	if err != nil {
		return false, err
	}

	return true, d.cacheResult(deployedObj, origObj)
}

func (d *Deployer) stampMetadata(obj *unstructured.Unstructured, input DeployInput, fo string) {
	resources.SetLabels(obj, d.labels)
	resources.SetAnnotations(obj, d.annotations)

	if input.Owner != nil {
		resources.SetAnnotation(obj, annotations.InstanceGeneration, strconv.FormatInt(input.Owner.GetGeneration(), 10))
		resources.SetAnnotation(obj, annotations.InstanceName, input.Owner.GetName())
		resources.SetAnnotation(obj, annotations.InstanceUID, string(input.Owner.GetUID()))
	}

	if input.Release.Type != "" {
		resources.SetAnnotation(obj, annotations.PlatformType, input.Release.Type)
	}

	if input.Release.Version != "" {
		resources.SetAnnotation(obj, annotations.PlatformVersion, input.Release.Version)
	}

	if resources.GetLabel(obj, labels.PlatformPartOf) == "" && fo != "" {
		resources.SetLabel(obj, labels.PlatformPartOf, fo)
	}
}

func (d *Deployer) applyResource(
	ctx context.Context,
	input DeployInput,
	obj *unstructured.Unstructured,
	current *unstructured.Unstructured,
	fo string,
) (*unstructured.Unstructured, error) {
	if resources.GetAnnotation(obj, d.managedKey) == "false" {
		resources.RemoveAnnotation(obj, d.managedKey)

		deployed, err := d.create(ctx, input.Client, obj)
		if err != nil && !k8serr.IsAlreadyExists(err) {
			return nil, err
		}

		return deployed, nil
	}

	if input.Owner != nil {
		obj.SetOwnerReferences(nil)

		err := ctrl.SetControllerReference(input.Owner, obj, input.Client.Scheme())
		if err != nil {
			return nil, err
		}
	}

	return d.applyWithMode(ctx, input.Client, obj, current, fo)
}

func (d *Deployer) applyWithMode(
	ctx context.Context,
	cli client.Client,
	obj *unstructured.Unstructured,
	current *unstructured.Unstructured,
	fieldOwner string,
) (*unstructured.Unstructured, error) {
	switch d.mode {
	case ModePatch:
		return d.patch(ctx, cli, obj, current, client.ForceOwnership, client.FieldOwner(fieldOwner))
	case ModeSSA:
		return d.apply(ctx, cli, obj, current, client.ForceOwnership, client.FieldOwner(fieldOwner))
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedMode, d.mode)
	}
}

func (d *Deployer) cacheResult(deployed, orig *unstructured.Unstructured) error {
	if d.cache == nil {
		return nil
	}

	err := d.cache.Add(deployed, orig)
	if err != nil {
		return fmt.Errorf("failed to cache object: %w", err)
	}

	return nil
}

func (d *Deployer) create(
	ctx context.Context,
	cli client.Client,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	logf.FromContext(ctx).V(3).Info("create",
		"gvk", obj.GroupVersionKind(),
		"name", client.ObjectKeyFromObject(obj),
	)

	err := cli.Create(ctx, obj)
	if err != nil {
		return obj, err
	}

	return obj, nil
}

func (d *Deployer) patch(
	ctx context.Context,
	cli client.Client,
	obj *unstructured.Unstructured,
	old *unstructured.Unstructured,
	opts ...client.PatchOption,
) (*unstructured.Unstructured, error) {
	logf.FromContext(ctx).V(3).Info("patch",
		"gvk", obj.GroupVersionKind(),
		"name", client.ObjectKeyFromObject(obj),
	)

	err := d.runMergeStrategy(old, obj)
	if err != nil {
		return nil, err
	}

	if old == nil {
		err = cli.Create(ctx, obj)
		if err != nil {
			return nil, fmt.Errorf("failed to create object %s/%s: %w",
				obj.GetNamespace(), obj.GetName(), err)
		}

		return obj, nil
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal apply patch for %s/%s: %w",
			obj.GetNamespace(), obj.GetName(), err)
	}

	err = cli.Patch(ctx, old, client.RawPatch(types.ApplyPatchType, data), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to patch object %s/%s: %w",
			obj.GetNamespace(), obj.GetName(), err)
	}

	return old, nil
}

func (d *Deployer) apply(
	ctx context.Context,
	cli client.Client,
	obj *unstructured.Unstructured,
	old *unstructured.Unstructured,
	opts ...client.ApplyOption,
) (*unstructured.Unstructured, error) {
	logf.FromContext(ctx).V(3).Info("apply",
		"gvk", obj.GroupVersionKind(),
		"name", client.ObjectKeyFromObject(obj),
	)

	err := d.runMergeStrategy(old, obj)
	if err != nil {
		return nil, err
	}

	if old != nil {
		_, found, nestedErr := unstructured.NestedFieldNoCopy(obj.Object, "aggregationRule")
		if nestedErr != nil {
			return nil, fmt.Errorf("failed to inspect aggregationRule for %s/%s: %w",
				obj.GetNamespace(), obj.GetName(), nestedErr)
		}

		if found {
			unstructured.RemoveNestedField(obj.Object, "rules")
		}
	}

	err = resources.Apply(ctx, cli, obj, opts...)
	if err != nil {
		return nil, fmt.Errorf("apply failed %s: %w", obj.GroupVersionKind(), err)
	}

	return obj, nil
}

func (d *Deployer) runMergeStrategy(old, obj *unstructured.Unstructured) error {
	if old == nil {
		return nil
	}

	mergeFn, ok := d.mergeStrategies[obj.GroupVersionKind()]
	if !ok {
		return nil
	}

	managed := resources.GetAnnotation(old, d.managedKey)
	if managed == "true" {
		return nil
	}

	err := mergeFn(old, obj)
	if err != nil {
		return fmt.Errorf("failed to merge %s %s/%s: %w",
			obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}
