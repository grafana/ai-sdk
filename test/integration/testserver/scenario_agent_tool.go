package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("agent-tool", handleAgentTool)
}

type agentToolModel struct {
	callCount int
}

func (m *agentToolModel) SpecificationVersion() string               { return "v4" }
func (m *agentToolModel) Provider() string                           { return "test" }
func (m *agentToolModel) ModelID() string                            { return "test-agent-tool" }
func (m *agentToolModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *agentToolModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *agentToolModel) DoStream(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	m.callCount++
	stream := make(chan provider.StreamPart, 4)
	if m.callCount == 1 {
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "weather-1", ToolName: "get_weather", Input: `{"city":"Paris"}`}
		stream <- provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls},
			Usage:        &provider.Usage{},
		}
	} else {
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "Paris is 18°C and partly cloudy."}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
		stream <- provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
			Usage:        &provider.Usage{},
		}
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

type integrationWeatherInput struct {
	City string `json:"city" jsonschema:"description=City to look up the weather for"`
}

type integrationWeatherOutput struct {
	City       string `json:"city"`
	Celsius    int    `json:"celsius"`
	Conditions string `json:"conditions"`
}

func handleAgentTool(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Messages []aisdk.UIMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	weather, err := aisdk.TypedTool(aisdk.TypedToolDef[integrationWeatherInput, integrationWeatherOutput]{
		Name:        "get_weather",
		Description: "Get the current weather for a city.",
		Execute: func(_ context.Context, input integrationWeatherInput, _ aisdk.ToolExecutionOptions) (integrationWeatherOutput, error) {
			time.Sleep(100 * time.Millisecond)
			return integrationWeatherOutput{City: input.City, Celsius: 18, Conditions: "partly cloudy"}, nil
		},
	})
	if err != nil {
		http.Error(w, "creating tool", http.StatusInternalServerError)
		return
	}

	agent := aisdk.NewToolLoopAgent(&agentToolModel{},
		aisdk.WithToolLoopAgentOptions(
			aisdk.WithInstructions("Use the weather tool when needed."),
			aisdk.WithTools(aisdk.ToolSet{"get_weather": weather}),
			aisdk.WithStopWhen(aisdk.StepCountIs(5)),
		),
	)
	if err := aisdk.WriteAgentUIStream(w, r.Context(), agent, body.Messages); err != nil {
		http.Error(w, "streaming response", http.StatusInternalServerError)
	}
}
