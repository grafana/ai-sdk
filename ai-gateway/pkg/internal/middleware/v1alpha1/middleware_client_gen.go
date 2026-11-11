package v1alpha1

import (
	"context"

	"github.com/grafana/grafana-app-sdk/resource"
)

type MiddlewareClient struct {
	client *resource.TypedClient[*Middleware, *MiddlewareList]
}

func NewMiddlewareClient(client resource.Client) *MiddlewareClient {
	return &MiddlewareClient{
		client: resource.NewTypedClient[*Middleware, *MiddlewareList](client, Kind()),
	}
}

func NewMiddlewareClientFromGenerator(generator resource.ClientGenerator) (*MiddlewareClient, error) {
	c, err := generator.ClientFor(Kind())
	if err != nil {
		return nil, err
	}
	return NewMiddlewareClient(c), nil
}

func (c *MiddlewareClient) Get(ctx context.Context, identifier resource.Identifier) (*Middleware, error) {
	return c.client.Get(ctx, identifier)
}

func (c *MiddlewareClient) List(ctx context.Context, namespace string, opts resource.ListOptions) (*MiddlewareList, error) {
	return c.client.List(ctx, namespace, opts)
}

func (c *MiddlewareClient) ListAll(ctx context.Context, namespace string, opts resource.ListOptions) (*MiddlewareList, error) {
	resp, err := c.client.List(ctx, namespace, resource.ListOptions{
		ResourceVersion: opts.ResourceVersion,
		Limit:           opts.Limit,
		LabelFilters:    opts.LabelFilters,
		FieldSelectors:  opts.FieldSelectors,
	})
	if err != nil {
		return nil, err
	}
	for resp.GetContinue() != "" {
		page, err := c.client.List(ctx, namespace, resource.ListOptions{
			Continue:        resp.GetContinue(),
			ResourceVersion: opts.ResourceVersion,
			Limit:           opts.Limit,
			LabelFilters:    opts.LabelFilters,
			FieldSelectors:  opts.FieldSelectors,
		})
		if err != nil {
			return nil, err
		}
		resp.SetContinue(page.GetContinue())
		resp.SetResourceVersion(page.GetResourceVersion())
		resp.SetItems(append(resp.GetItems(), page.GetItems()...))
	}
	return resp, nil
}

func (c *MiddlewareClient) Create(ctx context.Context, obj *Middleware, opts resource.CreateOptions) (*Middleware, error) {
	// Make sure apiVersion and kind are set
	obj.APIVersion = GroupVersion.Identifier()
	obj.Kind = Kind().Kind()
	return c.client.Create(ctx, obj, opts)
}

func (c *MiddlewareClient) Update(ctx context.Context, obj *Middleware, opts resource.UpdateOptions) (*Middleware, error) {
	return c.client.Update(ctx, obj, opts)
}

func (c *MiddlewareClient) Patch(ctx context.Context, identifier resource.Identifier, req resource.PatchRequest, opts resource.PatchOptions) (*Middleware, error) {
	return c.client.Patch(ctx, identifier, req, opts)
}

func (c *MiddlewareClient) Delete(ctx context.Context, identifier resource.Identifier, opts resource.DeleteOptions) error {
	return c.client.Delete(ctx, identifier, opts)
}
