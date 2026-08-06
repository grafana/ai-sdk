package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/failure"
)

// ModelResolver resolves a model using the complete normalized gateway call.
type ModelResolver interface {
	ResolveModel(ctx context.Context, call GatewayCall) (catalog.ResolvedModel, error)
}

// ModelResolverFunc adapts a function to ModelResolver.
type ModelResolverFunc func(ctx context.Context, call GatewayCall) (catalog.ResolvedModel, error)

// ResolveModel implements ModelResolver.
func (f ModelResolverFunc) ResolveModel(ctx context.Context, call GatewayCall) (catalog.ResolvedModel, error) {
	return f(ctx, call)
}

type catalogResolver struct {
	resolver catalog.ModelResolver
}

// AdaptCatalogResolver adapts a model-ID-only catalog resolver to normalized
// calls that contain no gateway routing controls.
func AdaptCatalogResolver(resolver catalog.ModelResolver) (ModelResolver, error) {
	if isNilInterface(resolver) {
		return nil, fmt.Errorf("gateway runtime: nil catalog resolver")
	}
	return &catalogResolver{resolver: resolver}, nil
}

func (resolver *catalogResolver) ResolveModel(ctx context.Context, call GatewayCall) (catalog.ResolvedModel, error) {
	if !call.GatewayOptions.Empty() {
		return catalog.ResolvedModel{}, unsupportedGatewayOptionsError()
	}
	resolved, err := resolver.resolver.ResolveModel(ctx, call.RequestedModelID)
	if err != nil {
		if errors.Is(err, catalog.ErrUnknownModel) {
			return catalog.ResolvedModel{}, failure.Wrap(failure.ErrUnknownModel, err)
		}
		return catalog.ResolvedModel{}, err
	}
	return resolved, nil
}
