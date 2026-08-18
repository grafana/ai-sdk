package middleware

import (
	"context"

	"github.com/grafana/ai-sdk/provider"
)

// DefaultInstructions returns a Middleware that prepends the provided system
// messages when a call does not already contain a system message.
func DefaultInstructions(instructions ...provider.Message) Middleware {
	defaults := append([]provider.Message(nil), instructions...)
	for i := range defaults {
		defaults[i].Role = provider.RoleSystem
	}
	return Middleware{
		TransformParams: func(_ context.Context, input TransformParamsInput) (provider.CallOptions, error) {
			if len(defaults) == 0 {
				return input.Params, nil
			}
			for _, message := range input.Params.Prompt {
				if message.Role == provider.RoleSystem {
					return input.Params, nil
				}
			}

			params := input.Params
			params.Prompt = append(append([]provider.Message(nil), defaults...), params.Prompt...)
			return params, nil
		},
	}
}
