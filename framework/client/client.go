package client

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var _ client.Client = (*Client)(nil)

// Client wraps a controller-runtime client to use unstructured resources
// for Get and List operations. This ensures a unified caching strategy
// where all resources go through the unstructured cache path.
type Client struct {
	inner client.Client
}

// New creates a Client that wraps the given client.
func New(inner client.Client) *Client {
	return &Client{inner: inner}
}

func (c *Client) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK: %w", err)
	}

	if _, isUnstructured := obj.(*unstructured.Unstructured); isUnstructured {
		return c.inner.Get(ctx, key, obj, opts...)
	}

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
	gvk, err := apiutil.GVKForObject(list, c.Scheme())
	if err != nil {
		return fmt.Errorf("failed to get GVK for list: %w", err)
	}

	if _, isUnstructuredList := list.(*unstructured.UnstructuredList); isUnstructuredList {
		return c.inner.List(ctx, list, opts...)
	}

	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(gvk)

	if err := c.inner.List(ctx, ul, opts...); err != nil {
		return err
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(ul.UnstructuredContent(), list); err != nil {
		return fmt.Errorf("failed to convert unstructured to typed: %w", err)
	}

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
