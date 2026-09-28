package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProgrammaticDenial_RequestConversion(t *testing.T) {
	for _, tc := range []struct {
		name, caller string
		assistant    bool
		custom       bool
		wantError    bool
	}{
		{name: "preceding program call", caller: "program", assistant: true, wantError: true},
		{name: "result caller without assistant", caller: "program", wantError: true},
		{name: "direct caller", caller: "direct", assistant: true},
		{name: "plain denied result", assistant: true},
		{name: "custom tool denial remains supported", caller: "program", custom: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := []provider.Message{provider.UserText("hi")}
			if tc.assistant {
				call := provider.ToolCallPart("call_1", "lookup", json.RawMessage(`{}`))
				if tc.caller != "" {
					call.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{Caller: &OpenAIToolCaller{Type: OpenAIToolCallerType(tc.caller), CallerID: "prog_1"}})
				}
				messages = append(messages, provider.NewAssistantMessage(call))
			}
			result := provider.ToolResultPart("call_1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied})
			if !tc.assistant && tc.caller != "" {
				result.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{Caller: &OpenAIToolCaller{Type: OpenAIToolCallerType(tc.caller), CallerID: "prog_1"}})
			}
			messages = append(messages, provider.NewToolMessage(result))
			tools := []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup"}}
			if tc.custom {
				tools[0] = provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "lookup"}
			}
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				contentType, payload := "application/json", `{"id":"resp_1","status":"completed","output":[]}`
				if req.Header.Get("Accept") == "text/event-stream" {
					contentType = "text/event-stream"
					payload = "event: response.completed\n" + `data: {"type":"response.completed","sequence_number":0,"response":{"id":"resp_1","status":"completed","output":[]}}` + "\n\n"
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload)), Request: req}, nil
			})}
			model := NewResponses("test-key", "gpt-6", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
			opts := provider.CallOptions{Prompt: messages, Tools: tools}
			_, generateErr := model.DoGenerate(t.Context(), opts)
			stream, streamErr := model.DoStream(t.Context(), opts)
			if tc.wantError {
				require.ErrorContains(t, generateErr, "execution-denied results for programmatic tool calls")
				require.ErrorContains(t, streamErr, "execution-denied results for programmatic tool calls")
				assert.Equal(t, 0, calls)
			} else {
				require.NoError(t, generateErr)
				require.NoError(t, streamErr)
				for range stream.Stream {
				}
				assert.Equal(t, 2, calls)
			}
		})
	}
}

func TestBuildParams_ProviderExecutedDenialIsNotHistory(t *testing.T) {
	part := provider.ToolResultPart("call_1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied})
	part.ProviderExecuted = true
	body, warnings := buildBody(t, "gpt-6", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(part)}})
	assert.Empty(t, warnings)
	assert.Empty(t, body["input"])
}
