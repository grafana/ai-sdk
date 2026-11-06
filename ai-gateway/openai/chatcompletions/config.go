// Package chatcompletions adapts an OpenAI Chat Completions subset to the provider domain.
package chatcompletions

import (
	"errors"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

type handler struct {
	runtime *execution.Runtime
	storage *StorageConfig
}

// New constructs a Chat Completions adapter over the host runtime.
func New(runtime *execution.Runtime, options ...Option) (*handler, error) {
	if runtime == nil {
		return nil, errors.New("chat completions: runtime is nil")
	}
	h := &handler{runtime: runtime}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("chat completions: nil option")
		}
		if err := option(h); err != nil {
			return nil, err
		}
	}
	return h, nil
}
