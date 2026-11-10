package service

import (
	"context"
	"errors"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/routing"
	"github.com/grafana/ai-sdk/provider"
)

var errMissingRoute = errors.New("gateway service: resolved model has no configured route")

// routedGroup is one model of a routed chain: the name it was requested by,
// its canonical ID and its candidates in configured order.
type routedGroup struct {
	name       string
	canonical  string
	candidates []catalog.ConfiguredCandidate
}

// resolveGroups resolves the requested model and each models entry, in order.
// An unknown requested model fails with resolve's error, and an unknown models
// entry with [execution.ErrUnknownFallbackModel].
func resolveGroups(id string, models []string, resolve func(name string) (routedGroup, error)) ([]routedGroup, error) {
	requested, err := resolve(id)
	if err != nil {
		return nil, err
	}
	groups := make([]routedGroup, 0, len(models)+1)
	groups = append(groups, requested)
	for _, name := range models {
		group, err := resolve(name)
		switch {
		case errors.Is(err, catalog.ErrUnknownModel):
			return nil, execution.ErrUnknownFallbackModel
		case err != nil:
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

// planRoute composes the attempt chain of groups under controls' only and
// order, bounded by the provider attempt bound, and returns its placements
// with the attempt expected at each.
func planRoute(groups []routedGroup, controls routing.Controls) ([]routing.Placement, []execution.ExpectedAttempt, error) {
	routingGroups := make([]routing.Group, len(groups))
	for i, group := range groups {
		routingGroups[i] = routing.Group{Key: group.canonical, Candidates: make([]routing.Candidate, len(group.candidates))}
		for j, identity := range group.candidates {
			routingGroups[i].Candidates[j] = routing.Candidate{Provider: identity.Provider, Destination: identity.ProviderInstance + "\x00" + identity.ModelID}
		}
	}
	placements, err := routing.Compose(routingGroups, controls.Only, controls.Order, config.MaxProviderAttempts)
	if err != nil {
		return nil, nil, err
	}
	expected := make([]execution.ExpectedAttempt, len(placements))
	for i, placement := range placements {
		group := groups[placement.Group]
		expected[i] = execution.ExpectedAttempt{ModelID: group.name, CanonicalID: group.canonical, Destination: group.candidates[placement.Candidate]}
	}
	return placements, expected, nil
}

// selectRouted composes one request chain from the configured candidates of
// the requested model and each fallback model, filtered and ordered by
// controls. It resolves every name through the catalog, so callers can name
// only public IDs and aliases, and uses only the startup-constructed
// candidates. The logical model keeps the requested model's canonical ID.
func (c *Catalog) selectRouted(ctx context.Context, id string, controls routing.Controls) (execution.Selection, error) {
	groups, err := resolveGroups(id, controls.Models, func(name string) (routedGroup, error) {
		model, err := c.ResolveModel(ctx, name)
		if err != nil {
			return routedGroup{}, err
		}
		if route, ok := c.routes[model.ID]; !ok || len(route.candidates) != len(model.Candidates) {
			return routedGroup{}, errMissingRoute
		}
		return routedGroup{name: name, canonical: model.ID, candidates: model.Candidates}, nil
	})
	if err != nil {
		return execution.Selection{}, err
	}
	placements, expected, err := planRoute(groups, controls)
	if err != nil {
		return execution.Selection{}, err
	}
	candidates := make([]provider.LanguageModel, 0, len(placements))
	descriptors := make([]config.Primary, 0, len(placements))
	for _, placement := range placements {
		route := c.routes[groups[placement.Group].canonical]
		candidates = append(candidates, route.candidates[placement.Candidate])
		descriptors = append(descriptors, route.descriptors[placement.Candidate])
	}
	lower, err := composeCandidates(candidates, descriptors, c.physical)
	if err != nil {
		return execution.Selection{}, err
	}
	model, err := c.factory(groups[0].canonical, lower)
	if err != nil {
		return execution.Selection{}, err
	}
	return execution.ConfiguredSelection(groups[0].canonical, model, expected), nil
}
