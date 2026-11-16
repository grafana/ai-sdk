package v1alpha1

import (
	"context"

	"github.com/grafana/grafana-app-sdk/resource"
)

type ModelCatalogClient struct {
	client *resource.TypedClient[*ModelCatalog, *ModelCatalogList]
}

func NewModelCatalogClient(client resource.Client) *ModelCatalogClient {
	return &ModelCatalogClient{
		client: resource.NewTypedClient[*ModelCatalog, *ModelCatalogList](client, Kind()),
	}
}

func NewModelCatalogClientFromGenerator(generator resource.ClientGenerator) (*ModelCatalogClient, error) {
	c, err := generator.ClientFor(Kind())
	if err != nil {
		return nil, err
	}
	return NewModelCatalogClient(c), nil
}

func (c *ModelCatalogClient) Get(ctx context.Context, identifier resource.Identifier) (*ModelCatalog, error) {
	return c.client.Get(ctx, identifier)
}

func (c *ModelCatalogClient) List(ctx context.Context, namespace string, opts resource.ListOptions) (*ModelCatalogList, error) {
	return c.client.List(ctx, namespace, opts)
}

func (c *ModelCatalogClient) ListAll(ctx context.Context, namespace string, opts resource.ListOptions) (*ModelCatalogList, error) {
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

func (c *ModelCatalogClient) Create(ctx context.Context, obj *ModelCatalog, opts resource.CreateOptions) (*ModelCatalog, error) {
	// Make sure apiVersion and kind are set
	obj.APIVersion = GroupVersion.Identifier()
	obj.Kind = Kind().Kind()
	return c.client.Create(ctx, obj, opts)
}

func (c *ModelCatalogClient) Update(ctx context.Context, obj *ModelCatalog, opts resource.UpdateOptions) (*ModelCatalog, error) {
	return c.client.Update(ctx, obj, opts)
}

func (c *ModelCatalogClient) Patch(ctx context.Context, identifier resource.Identifier, req resource.PatchRequest, opts resource.PatchOptions) (*ModelCatalog, error) {
	return c.client.Patch(ctx, identifier, req, opts)
}

func (c *ModelCatalogClient) Delete(ctx context.Context, identifier resource.Identifier, opts resource.DeleteOptions) error {
	return c.client.Delete(ctx, identifier, opts)
}
