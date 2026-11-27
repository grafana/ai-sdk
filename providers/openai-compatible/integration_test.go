package openaicompatible

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2EStreamText_SyntheticToolResultContinuation(t *testing.T) {
	t.Parallel()

	responses := []string{
		`data: {"id":"step-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-sample","type":"function","function":{"name":"lookup_sample_code","arguments":"{\"sample\":"}}]}}]}

data: {"id":"step-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"circle\"}"}}]}}]}

data: {"id":"step-1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`,
		`data: {"id":"step-2","choices":[{"index":0,"delta":{"content":"violet-42"}}]}

data: {"id":"step-2","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`,
	}
	type chatRequest struct {
		Messages json.RawMessage `json:"messages"`
		Stream   bool            `json:"stream"`
	}
	var requests []chatRequest
	const baseURL = "https://provider.invalid/v1"
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		defer func() { assert.NoError(t, req.Body.Close()) }()
		assert.Equal(t, http.MethodPost, req.Method)
		assert.Equal(t, baseURL+"/chat/completions", req.URL.String())
		var body chatRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, fmt.Errorf("synthetic chat: decoding request: %w", err)
		}
		index := len(requests)
		requests = append(requests, body)
		if index >= len(responses) {
			return nil, fmt.Errorf("synthetic chat: unexpected request %d", index+1)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(responses[index])),
		}, nil
	})}
	sampleSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"sample":{"type":"string"}},"required":["sample"]}`))
	require.NoError(t, err)
	var executions atomic.Int32
	model := New("test-model", WithBaseURL(baseURL), WithHTTPClient(client))
	result := aisdk.StreamText(t.Context(), model,
		aisdk.WithModelMessages(provider.UserText("Look up the code for circle.")),
		aisdk.WithMaxRetries(0),
		aisdk.WithStopWhen(aisdk.StepCountIs(2)),
		aisdk.WithTools(aisdk.ToolSet{"lookup_sample_code": {
			InputSchema: sampleSchema,
			Execute: func(_ context.Context, input json.RawMessage, opts aisdk.ToolExecutionOptions) (json.RawMessage, error) {
				executions.Add(1)
				assert.Equal(t, "call-sample", opts.ToolCallID)
				assert.JSONEq(t, `{"sample":"circle"}`, string(input))
				return json.RawMessage(`{"code":"violet-42"}`), nil
			},
		}}),
	)
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())

	require.Len(t, requests, 2)
	assert.True(t, requests[0].Stream)
	assert.True(t, requests[1].Stream)
	assert.JSONEq(t, `[{"role":"user","content":"Look up the code for circle."}]`, string(requests[0].Messages))
	assert.JSONEq(t, `[
		{"role":"user","content":"Look up the code for circle."},
		{"role":"assistant","content":null,"tool_calls":[{"id":"call-sample","type":"function","function":{"name":"lookup_sample_code","arguments":"{\"sample\":\"circle\"}"}}]},
		{"role":"tool","tool_call_id":"call-sample","content":"{\"code\":\"violet-42\"}"}
	]`, string(requests[1].Messages))

	assert.EqualValues(t, 1, executions.Load())
	toolResults := result.ToolResults()
	require.Len(t, toolResults, 1)
	assert.Equal(t, "call-sample", toolResults[0].ToolCallID)
	assert.Equal(t, "lookup_sample_code", toolResults[0].ToolName)
	assert.False(t, toolResults[0].IsError)
	assert.JSONEq(t, `{"code":"violet-42"}`, string(toolResults[0].Output))

	steps := result.Steps()
	require.Len(t, steps, 2)
	assert.Equal(t, provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: "tool_calls"}, steps[0].FinishReason)
	assert.Equal(t, provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "stop"}, steps[1].FinishReason)
	assert.Equal(t, "violet-42", result.Text())
}
