package main

import (
	"context"
	"errors"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

const startupAbortReason = "startup canceled"

func init() {
	for _, phase := range []string{"stream-start-only", "prepare-step-abort", "do-stream-abort"} {
		registerScenario(phase, func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithCancelCause(r.Context())
			defer cancel(nil)
			abort := func() { cancel(errors.New(startupAbortReason)) }

			model := &startupModel{}
			opts := []aisdk.StreamOption{
				aisdk.WithModelMessages(provider.UserText("hi")),
				aisdk.WithMaxRetries(0),
			}
			switch phase {
			case "prepare-step-abort":
				opts = append(opts, aisdk.WithPrepareStep(func(aisdk.PrepareStepState) (*aisdk.PrepareStepResult, error) {
					abort()
					return nil, ctx.Err()
				}))
			case "do-stream-abort":
				model.abort = abort
			}
			result := aisdk.StreamText(ctx, model, opts...)
			if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})
	}
}

type startupModel struct {
	simpleTextModel
	abort func()
}

func (m *startupModel) DoStream(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	if m.abort != nil {
		m.abort()
		return nil, ctx.Err()
	}
	stream := make(chan provider.StreamPart, 1)
	stream <- provider.StreamPart{Type: provider.PartStreamStart}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}
