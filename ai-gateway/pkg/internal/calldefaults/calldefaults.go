package calldefaults

import (
	"context"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

// Definitions returns the definitions of the shipped tenant extensions.
func Definitions() []extension.Definition {
	return []extension.Definition{instructionsDefinition(), settingsDefinition()}
}

// transformer runs a model middleware's parameter transform as a request
// transform.
type transformer struct {
	middleware middleware.Middleware
}

func (t transformer) TransformRequest(ctx context.Context, _ *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
	return t.middleware.TransformParams(ctx, middleware.TransformParamsInput{Params: params})
}
