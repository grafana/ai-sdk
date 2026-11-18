package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/option"
	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/grafana/ai-sdk/schema"
)

func init() {
	registerScenario("anthropic-json-tool-fallback", handleAnthropicJSONToolFallback)
}

type jsonFallbackTransport struct{}

func (jsonFallbackTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var request struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		ToolChoice struct {
			Type                   provider.ToolChoiceType `json:"type"`
			DisableParallelToolUse bool                    `json:"disable_parallel_tool_use"`
		} `json:"tool_choice"`
	}
	decodeErr := json.NewDecoder(r.Body).Decode(&request)
	closeErr := r.Body.Close()
	if decodeErr != nil {
		return nil, fmt.Errorf("decoding Anthropic test request: %w", decodeErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("closing Anthropic test request: %w", closeErr)
	}
	if len(request.Tools) != 1 || request.Tools[0].Name != "json" ||
		request.ToolChoice.Type != provider.ToolChoiceAuto || !request.ToolChoice.DisableParallelToolUse {
		return nil, fmt.Errorf("anthropic test request: want only the JSON tool with auto choice and parallel tool use disabled, got %+v", request)
	}

	const sse = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-3-haiku","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"{\"answer\":\"42\"}"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"citations_delta","citation":{"type":"web_search_result_location","url":"https://example.com/report","title":"Report","cited_text":"source","encrypted_index":"opaque"}}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}

event: message_stop
data: {"type":"message_stop"}

`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
		Request:    r,
	}, nil
}

func handleAnthropicJSONToolFallback(w http.ResponseWriter, r *http.Request) {
	type answer struct {
		Answer string `json:"answer"`
	}
	s, err := schema.SchemaFor[answer]()
	if err != nil {
		http.Error(w, fmt.Sprintf("building test schema: %v", err), http.StatusInternalServerError)
		return
	}
	out, err := output.Object[answer](s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	client := &http.Client{Transport: jsonFallbackTransport{}}
	model := anthropic.New("test-key", "claude-3-haiku",
		anthropic.WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)),
	)
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("Return a JSON answer.")),
		aisdk.WithOutput(out),
		aisdk.WithProviderOptions(anthropic.AnthropicOptions{
			Thinking: &anthropic.ThinkingConfig{Type: anthropic.ThinkingEnabled, BudgetTokens: 1024},
		}),
	)
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
