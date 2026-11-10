package service

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/grafana/grafana-app-sdk/resource"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/routing"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/stackcatalog"
	"github.com/grafana/ai-sdk/provider"
)

// StackCatalog is one stored stack catalog as a request supplies it.
type StackCatalog struct {
	// Name is the catalog's object name.
	Name string
	// Spec is the stored spec, checked again on every selection.
	Spec v1alpha1.Spec
	// Keys are the catalog's decrypted secure values. Each key is a provider
	// name.
	Keys resource.DecryptedSecureValues
}

// StackCatalogOptions configures stack-catalog selection.
type StackCatalogOptions struct {
	// Client is the shared model HTTP client.
	Client *http.Client
}

// candidateBuilder constructs the physical model of one catalog candidate.
type candidateBuilder func(providerConfig config.ResolvedProvider, descriptor config.Primary) (provider.LanguageModel, error)

// SelectStackCatalog selects modelID from one stack catalog for one request.
// It refuses BYOK and any other unsupported gateway control, checks the stored
// catalog, failing with [execution.ErrCatalogConfiguration] without its
// reason, resolves the requested model and models entries within the catalog,
// and composes the routed chain. Only the chain's candidates are constructed,
// with the catalog's keys, and no physical attempt records are written. The
// selection carries the catalog's name and keys.
func SelectStackCatalog(stack StackCatalog, modelID string, gateway json.RawMessage, options StackCatalogOptions) (execution.Selection, error) {
	if options.Client == nil {
		return execution.Selection{}, fmt.Errorf("gateway service: model HTTP client is required")
	}
	return selectStackCatalog(stack, modelID, gateway, func(providerConfig config.ResolvedProvider, descriptor config.Primary) (provider.LanguageModel, error) {
		candidate, _, err := newCandidate(providerConfig, descriptor, options.Client, nativemodel.NewAnthropic)
		return candidate, err
	})
}

func selectStackCatalog(stack StackCatalog, modelID string, gateway json.RawMessage, build candidateBuilder) (execution.Selection, error) {
	controls, err := routing.Decode(gateway, routing.ConfiguredAccess)
	if err != nil {
		return execution.Selection{}, err
	}
	secureKeys := make([]string, 0, len(stack.Keys))
	keys := make([]string, 0, len(stack.Keys))
	for name, value := range stack.Keys {
		if value.IsZero() {
			continue
		}
		secureKeys = append(secureKeys, name)
		keys = append(keys, string(value))
	}
	file, err := stackcatalog.Parse(stack.Spec, secureKeys)
	if err != nil {
		return execution.Selection{}, execution.ErrCatalogConfiguration
	}
	index := newStackIndex(file)
	groups, err := resolveGroups(modelID, controls.Models, index.resolve)
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
		descriptor := index.descriptors(groups[placement.Group].canonical)[placement.Candidate]
		providerConfig := config.ResolvedProvider{Type: file.Providers[descriptor.Provider].Type, APIKey: string(stack.Keys[descriptor.Provider])}
		candidate, err := build(providerConfig, descriptor)
		if err != nil {
			return execution.Selection{}, err
		}
		candidates = append(candidates, candidate)
		descriptors = append(descriptors, descriptor)
	}
	lower, err := composeCandidates(candidates, descriptors, nil)
	if err != nil {
		return execution.Selection{}, err
	}
	model, err := ConfiguredIdentity(groups[0].canonical, lower)
	if err != nil {
		return execution.Selection{}, err
	}
	return execution.StackCatalogSelection(groups[0].canonical, stack.Name, model, expected, keys), nil
}

// stackIndex resolves a checked stack catalog's public IDs without
// constructing any model.
type stackIndex struct {
	file      config.File
	canonical map[string]string
}

func newStackIndex(file config.File) stackIndex {
	canonical := make(map[string]string, len(file.Models))
	for id, model := range file.Models {
		canonical[id] = id
		for _, alias := range model.Aliases {
			canonical[alias] = id
		}
	}
	return stackIndex{file: file, canonical: canonical}
}

// descriptors returns a model's primary and fallbacks in configured order.
func (index stackIndex) descriptors(id string) []config.Primary {
	model := index.file.Models[id]
	return append([]config.Primary{model.Primary}, model.Fallback...)
}

func (index stackIndex) resolve(name string) (routedGroup, error) {
	id, ok := index.canonical[name]
	if !ok {
		return routedGroup{}, &catalog.UnknownModelError{ModelID: name}
	}
	descriptors := index.descriptors(id)
	candidates := make([]catalog.ConfiguredCandidate, len(descriptors))
	for i, descriptor := range descriptors {
		candidates[i] = catalog.ConfiguredCandidate{ProviderInstance: descriptor.Provider, Provider: index.file.Providers[descriptor.Provider].Type, ModelID: descriptor.Model}
	}
	return routedGroup{name: name, canonical: id, candidates: candidates}, nil
}
