package v1alpha1

import (
	"context"

	"github.com/grafana/grafana-app-sdk/resource"
)

type StoredChatCompletionClient struct {
	client *resource.TypedClient[*StoredChatCompletion, *StoredChatCompletionList]
}

func NewStoredChatCompletionClient(client resource.Client) *StoredChatCompletionClient {
	return &StoredChatCompletionClient{
		client: resource.NewTypedClient[*StoredChatCompletion, *StoredChatCompletionList](client, Kind()),
	}
}

func NewStoredChatCompletionClientFromGenerator(generator resource.ClientGenerator) (*StoredChatCompletionClient, error) {
	c, err := generator.ClientFor(Kind())
	if err != nil {
		return nil, err
	}
	return NewStoredChatCompletionClient(c), nil
}

func (c *StoredChatCompletionClient) Get(ctx context.Context, identifier resource.Identifier) (*StoredChatCompletion, error) {
	return c.client.Get(ctx, identifier)
}

func (c *StoredChatCompletionClient) List(ctx context.Context, namespace string, opts resource.ListOptions) (*StoredChatCompletionList, error) {
	return c.client.List(ctx, namespace, opts)
}

func (c *StoredChatCompletionClient) ListAll(ctx context.Context, namespace string, opts resource.ListOptions) (*StoredChatCompletionList, error) {
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

func (c *StoredChatCompletionClient) Create(ctx context.Context, obj *StoredChatCompletion, opts resource.CreateOptions) (*StoredChatCompletion, error) {
	// Make sure apiVersion and kind are set
	obj.APIVersion = GroupVersion.Identifier()
	obj.Kind = Kind().Kind()
	return c.client.Create(ctx, obj, opts)
}

func (c *StoredChatCompletionClient) Update(ctx context.Context, obj *StoredChatCompletion, opts resource.UpdateOptions) (*StoredChatCompletion, error) {
	return c.client.Update(ctx, obj, opts)
}

func (c *StoredChatCompletionClient) Patch(ctx context.Context, identifier resource.Identifier, req resource.PatchRequest, opts resource.PatchOptions) (*StoredChatCompletion, error) {
	return c.client.Patch(ctx, identifier, req, opts)
}

func (c *StoredChatCompletionClient) Delete(ctx context.Context, identifier resource.Identifier, opts resource.DeleteOptions) error {
	return c.client.Delete(ctx, identifier, opts)
}
