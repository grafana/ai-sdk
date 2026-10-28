// Package chatcompletions adapts an OpenAI Chat Completions subset to the provider domain.
package chatcompletions

import (
	"errors"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

type handler struct{ runtime *execution.Runtime }

// New constructs a Chat Completions adapter over the host runtime.
func New(runtime *execution.Runtime) (*handler, error) {
	if runtime == nil {
		return nil, errors.New("chat completions: runtime is nil")
	}
	return &handler{runtime: runtime}, nil
}
