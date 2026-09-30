package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

func init() { registerScenario("deferred-tool-discovery", handleDeferredToolDiscovery) }

type deferredToolDiscoveryModel struct{ step int }

func (*deferredToolDiscoveryModel) SpecificationVersion() string               { return "v4" }
func (*deferredToolDiscoveryModel) Provider() string                           { return "test" }
func (*deferredToolDiscoveryModel) ModelID() string                            { return "deferred-tool-discovery" }
func (*deferredToolDiscoveryModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*deferredToolDiscoveryModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, errors.New("discovery scenario uses streaming")
}
func (m *deferredToolDiscoveryModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 4)
	reason := provider.FinishReasonToolCalls
	switch m.step {
	case 0:
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "search", ToolName: "search", Input: `{"query":"weather"}`}
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "too-early", ToolName: "getWeather", Input: `{}`}
	case 1:
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "weather", ToolName: "getWeather", Input: `{}`}
	default:
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text"}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "sunny"}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text"}
		reason = provider.FinishReasonStop
	}
	m.step++
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: reason}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleDeferredToolDiscovery(w http.ResponseWriter, r *http.Request) {
	input, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result := aisdk.StreamText(r.Context(), &deferredToolDiscoveryModel{}, aisdk.WithModelMessages(provider.UserText("Find the weather.")), aisdk.WithStopWhen(aisdk.StepCountIs(3)), aisdk.WithTools(aisdk.ToolSet{
		"search": aisdk.ToolSearch(),
		"getWeather": {Description: "Weather forecast", DeferLoading: true, InputSchema: input, Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`"sunny"`), nil
		}},
	}))
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
