package service

import (
	"context"
	"errors"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/routing"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

var errMissingRoute = errors.New("gateway service: resolved model has no configured route")

// selectRouted composes one request chain from the configured candidates of
// the requested model and each fallback model, filtered and ordered by
// controls. It resolves every name through the catalog, so callers can name
// only public IDs and aliases, and uses only the startup-constructed
// candidates. The logical model keeps the requested model's canonical ID.
func (c *Catalog) selectRouted(ctx context.Context, id string, controls routing.Controls) (execution.Selection, error) {
	requested, err := c.ResolveModel(ctx, id)
	if err != nil {
		return execution.Selection{}, err
	}
	names := append([]string{id}, controls.Models...)
	resolved := make([]catalog.ResolvedModel, 0, len(names))
	resolved = append(resolved, requested)
	for _, name := range controls.Models {
		model, err := c.ResolveModel(ctx, name)
		switch {
		case errors.Is(err, catalog.ErrUnknownModel):
			return execution.Selection{}, execution.ErrUnknownFallbackModel
		case err != nil:
			return execution.Selection{}, err
		}
		resolved = append(resolved, model)
	}
	groups := make([]routing.Group, len(resolved))
	for i, model := range resolved {
		route, ok := c.routes[model.ID]
		if !ok || len(route.candidates) != len(model.Candidates) {
			return execution.Selection{}, errMissingRoute
		}
		groups[i] = routing.Group{Key: model.ID, Candidates: make([]routing.Candidate, len(model.Candidates))}
		for j, identity := range model.Candidates {
			groups[i].Candidates[j] = routing.Candidate{Provider: identity.Provider, Destination: identity.ProviderInstance + "\x00" + identity.ModelID}
		}
	}
	placements, err := routing.Compose(groups, controls.Only, controls.Order, config.MaxProviderAttempts)
	if err != nil {
		return execution.Selection{}, err
	}
	candidates := make([]provider.LanguageModel, 0, len(placements))
	descriptors := make([]config.Primary, 0, len(placements))
	expected := make([]execution.ExpectedAttempt, 0, len(placements))
	for _, placement := range placements {
		route := c.routes[resolved[placement.Group].ID]
		candidates = append(candidates, route.candidates[placement.Candidate])
		descriptors = append(descriptors, route.descriptors[placement.Candidate])
		expected = append(expected, execution.ExpectedAttempt{ModelID: names[placement.Group], CanonicalID: resolved[placement.Group].ID, Destination: resolved[placement.Group].Candidates[placement.Candidate]})
	}
	lower, err := composeCandidates(candidates, descriptors, c.physical)
	if err != nil {
		return execution.Selection{}, err
	}
	model, err := c.factory(requested.ID, lower)
	if err != nil {
		return execution.Selection{}, err
	}
	return execution.ConfiguredSelection(requested.ID, model, expected), nil
}
