package v4

import (
	"context"
	"encoding/json"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

func (h *handler) selectModel(ctx context.Context, modelID string, options provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
	if err := ctx.Err(); err != nil {
		return execution.Selection{}, err
	}
	type outcome struct {
		selection execution.Selection
		err       error
	}
	ready := make(chan outcome, 1)
	go func() {
		result := outcome{}
		defer func() {
			if recover() != nil {
				result = outcome{err: errRuntimeInternal}
			}
			ready <- result
		}()
		result.selection, result.err = h.selector(ctx, modelID, options, gateway)
		if result.err == nil && !validSelection(result.selection) {
			result = outcome{err: errRuntimeInternal}
		}
	}()
	select {
	case result := <-ready:
		if err := ctx.Err(); err != nil {
			return execution.Selection{}, err
		}
		return result.selection, result.err
	case <-ctx.Done():
		return execution.Selection{}, ctx.Err()
	}
}
