package main

import (
	"context"
	"net/http"
	"regexp"
	"time"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

const (
	controlledStreamStartDelay = 100 * time.Millisecond
	controlledStreamHold       = 500 * time.Millisecond
	controlledPartialText      = "Hello"
	controlledLaterText        = ", world!"
)

func init() {
	registerScenario("controlled-ui-stream", handleControlledUIStream)
	registerScenario("controlled-text-stream", handleControlledTextStream)
}

type controlledStreamModel struct{}

func (*controlledStreamModel) SpecificationVersion() string               { return "v4" }
func (*controlledStreamModel) Provider() string                           { return "test" }
func (*controlledStreamModel) ModelID() string                            { return "test-controlled-stream" }
func (*controlledStreamModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*controlledStreamModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*controlledStreamModel) DoStream(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 1)
	go func() {
		defer close(stream)
		if !sendProviderPart(ctx, stream, provider.StreamPart{Type: provider.PartTextStart, ID: "controlled-text"}) {
			return
		}
		if !sendProviderPart(ctx, stream, provider.StreamPart{Type: provider.PartTextDelta, ID: "controlled-text", Delta: controlledPartialText}) {
			return
		}
		if !waitForContext(ctx, controlledStreamHold) {
			return
		}
		if !sendProviderPart(ctx, stream, provider.StreamPart{Type: provider.PartTextDelta, ID: "controlled-text", Delta: controlledLaterText}) {
			return
		}
		if !sendProviderPart(ctx, stream, provider.StreamPart{Type: provider.PartTextEnd, ID: "controlled-text"}) {
			return
		}
		sendProviderPart(ctx, stream, provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
			Usage:        &provider.Usage{},
		})
	}()
	return &provider.StreamResult{Stream: stream}, nil
}

func handleControlledUIStream(w http.ResponseWriter, r *http.Request) {
	if !waitForContext(r.Context(), controlledStreamStartDelay) {
		return
	}
	result := aisdk.StreamText(r.Context(), &controlledStreamModel{},
		aisdk.WithModelMessages(provider.UserText("hello")),
	)
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleControlledTextStream(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &controlledStreamModel{},
		aisdk.WithModelMessages(provider.UserText("hello")),
	)
	if err := aisdk.WriteTextStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func waitForContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func sendProviderPart(ctx context.Context, stream chan<- provider.StreamPart, part provider.StreamPart) bool {
	select {
	case stream <- part:
		return true
	case <-ctx.Done():
		return false
	}
}
