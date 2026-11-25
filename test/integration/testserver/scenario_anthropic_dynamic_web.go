package main

import (
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
	registerScenario("anthropic-dynamic-web", handleAnthropicDynamicWeb)
}

type dynamicWebTransport struct{}

func (dynamicWebTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := r.Body.Close(); err != nil {
		return nil, fmt.Errorf("closing Anthropic test request: %w", err)
	}
	const sse = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"code-1","name":"code_execution","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"code_execution_tool_result","tool_use_id":"code-1","content":{"type":"code_execution_result","stdout":"","stderr":"","return_code":0,"content":[]}}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: content_block_start
data: {"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"{\"answer\":\"42\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":2}

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

func handleAnthropicDynamicWeb(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Transport: dynamicWebTransport{}}
	model := anthropic.New("test-key", "claude-sonnet-4-6",
		anthropic.WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)),
	)
	opts := []aisdk.StreamOption{
		aisdk.WithModelMessages(provider.UserText("Find the answer.")),
		aisdk.WithTools(aisdk.ToolSet{"search": {Type: aisdk.UserToolProvider, ID: "anthropic.web_search_20260318"}}),
	}
	if r.URL.Query().Get("fallback") == "true" {
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
			http.Error(w, fmt.Sprintf("building test output: %v", err), http.StatusInternalServerError)
			return
		}
		opts = append(opts, aisdk.WithOutput(out), aisdk.WithProviderOptions(anthropic.AnthropicOptions{
			StructuredOutputMode: anthropic.StructuredOutputJSONTool,
		}))
	}
	result := aisdk.StreamText(r.Context(), model, opts...)
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
