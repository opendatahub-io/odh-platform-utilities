package client

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Client wraps a controller-runtime client to force Get and List through
// unstructured resources, ensuring all cache reads hit the same informer
// the watches populate. Write operations delegate unchanged.
//
// controller-runtime maintains separate informer caches per GVK and per
// object type (typed vs unstructured). If a controller watches resources
// as unstructured but reads via typed Get/List, the read hits a different
// informer cache, causing stale reads, unnecessary API server calls, and
// doubled memory. This client solves the problem by transparently
// converting typed Get/List calls to unstructured under the hood.
type Client struct {
	inner client.Client
}

// Option configures the Client. Currently no options are defined;
// planned extensions include metadata-only caching via PartialObjectMetadata
// for watches/reads where only labels, annotations, and ownership are needed.
type Option func(*Client)

func New(inner client.Client, opts ...Option) *Client {
	c := &Client{
		inner: inner,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	log := logf.FromContext(ctx)

	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK: %w", err)
	}

	_, isUnstructured := obj.(*unstructured.Unstructured)
	_, isPartialMeta := obj.(*metav1.PartialObjectMetadata)

	if isUnstructured || isPartialMeta {
		log.V(1).Info("Client.Get: no conversion needed - input type matches cache type",
			"gvk", gvk, "key", key, "isUnstructured", isUnstructured, "isPartialMeta", isPartialMeta, "converted", false)
		return c.inner.Get(ctx, key, obj, opts...)
	}

	log.V(1).Info("Client.Get: typed input, non-exempted GVK - using unstructured cache with conversion",
		"gvk", gvk, "key", key, "inputType", "typed", "cacheType", "unstructured", "converted", true)

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)

	if err := c.inner.Get(ctx, key, u, opts...); err != nil {
		return err
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
		return fmt.Errorf("failed to convert unstructured to typed: %w", err)
	}

	obj.GetObjectKind().SetGroupVersionKind(gvk)

	return nil
}

func (c *Client) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	log := logf.FromContext(ctx)

	gvk, err := apiutil.GVKForObject(list, c.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK for list: %w", err)
	}

	_, isUnstructuredList := list.(*unstructured.UnstructuredList)
	_, isPartialMetaList := list.(*metav1.PartialObjectMetadataList)

	if isUnstructuredList || isPartialMetaList {
		log.V(1).Info("Client.List: no conversion needed - input type matches cache type",
			"gvk", gvk, "isUnstructuredList", isUnstructuredList, "isPartialMetaList", isPartialMetaList, "converted", false)
		return c.inner.List(ctx, list, opts...)
	}

	if hasFieldSelector(opts) {
		log.V(1).Info("Client.List: field selector detected - delegating to typed cache to preserve indexer compatibility",
			"gvk", gvk)
		return c.inner.List(ctx, list, opts...)
	}

	log.V(1).Info("Client.List: typed input, non-exempted GVK - using unstructured cache with conversion",
		"gvk", gvk, "inputType", "typed", "cacheType", "unstructured", "converted", true)

	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(gvk)

	if err := c.inner.List(ctx, ul, opts...); err != nil {
		return err
	}

	itemGVK := schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    strings.TrimSuffix(gvk.Kind, "List"),
	}

	items := make([]runtime.Object, 0, len(ul.Items))
	for i := range ul.Items {
		obj, err := c.Scheme().New(itemGVK)
		if err != nil {
			return fmt.Errorf("failed to create typed object for item %d: %w", i, err)
		}

		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(ul.Items[i].Object, obj); err != nil {
			return fmt.Errorf("failed to convert unstructured item %d to typed: %w", i, err)
		}

		ul.Items[i].Object = nil

		items = append(items, obj)
	}

	if err := meta.SetList(list, items); err != nil {
		return fmt.Errorf("failed to set typed list items: %w", err)
	}

	list.SetResourceVersion(ul.GetResourceVersion())
	list.SetContinue(ul.GetContinue())
	list.SetRemainingItemCount(ul.GetRemainingItemCount())
	list.GetObjectKind().SetGroupVersionKind(gvk)

	return nil
}

func (c *Client) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return c.inner.Create(ctx, obj, opts...)
}

func (c *Client) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return c.inner.Delete(ctx, obj, opts...)
}

func (c *Client) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return c.inner.Update(ctx, obj, opts...)
}

func (c *Client) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return c.inner.Patch(ctx, obj, patch, opts...)
}

func (c *Client) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	return c.inner.Apply(ctx, obj, opts...)
}

func (c *Client) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return c.inner.DeleteAllOf(ctx, obj, opts...)
}

func (c *Client) Status() client.SubResourceWriter {
	return c.inner.Status()
}

func (c *Client) SubResource(subResource string) client.SubResourceClient {
	return c.inner.SubResource(subResource)
}

func (c *Client) Scheme() *runtime.Scheme {
	return c.inner.Scheme()
}

func (c *Client) RESTMapper() meta.RESTMapper {
	return c.inner.RESTMapper()
}

func (c *Client) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return c.inner.GroupVersionKindFor(obj)
}

func (c *Client) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return c.inner.IsObjectNamespaced(obj)
}

func hasFieldSelector(opts []client.ListOption) bool {
	listOpts := &client.ListOptions{}
	for _, o := range opts {
		o.ApplyToList(listOpts)
	}

	return listOpts.FieldSelector != nil
}

var _ client.Client = (*Client)(nil)
