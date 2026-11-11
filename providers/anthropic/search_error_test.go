package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSearchError_Continuation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output json.RawMessage
	}{
		{name: "object", output: json.RawMessage(`{"errorCode":"invalid_tool_input"}`)},
		{name: "JSON string", output: json.RawMessage(`"{\"errorCode\":\"invalid_tool_input\"}"`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := provider.ToolResultPart("search_1", "web_search", &provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: tc.output})
			part.ProviderExecuted = true
			part.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"caller":{"type":"code_execution_20260120","toolId":"program_1"}}`)}}
			params, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(part)}}, false)
			require.NoError(t, err)
			require.Len(t, params.Messages, 1)
			encoded, err := json.Marshal(params.Messages[0].Content)
			require.NoError(t, err)
			assert.JSONEq(t, `[{"type":"web_search_tool_result","tool_use_id":"search_1","content":{"type":"web_search_tool_result_error","error_code":"invalid_tool_input"},"caller":{"type":"code_execution_20260120","tool_id":"program_1"}}]`, string(encoded))
		})
	}
}

func TestToolSearchError_Continuation(t *testing.T) {
	const (
		regexAlias = "search_regex"
		bm25Alias  = "search_bm25"
	)
	tools := []provider.Tool{
		{Type: provider.ToolTypeProvider, ID: "anthropic.tool_search_regex_20251119", Name: regexAlias},
		{Type: provider.ToolTypeProvider, ID: "anthropic.tool_search_bm25_20251119", Name: bm25Alias},
	}
	for _, tc := range []struct {
		name       string
		toolName   string
		output     provider.ToolResultOutput
		cacheMark  bool
		wireName   string
		wireResult string
	}{
		{
			name:       "error-json response shape",
			toolName:   regexAlias,
			output:     provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"type":"tool_search_tool_result_error","errorCode":"invalid_tool_input","errorMessage":null}`)},
			wireName:   "tool_search_tool_regex",
			wireResult: `{"type":"tool_search_tool_result_error","error_code":"invalid_tool_input"}`,
		},
		{
			name:       "error-json JSON string",
			toolName:   bm25Alias,
			output:     provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`"{\"type\":\"tool_search_tool_result_error\",\"errorCode\":\"too_many_requests\"}"`)},
			wireName:   "tool_search_tool_bm25",
			wireResult: `{"type":"tool_search_tool_result_error","error_code":"too_many_requests"}`,
		},
		{
			name:       "error-json without errorCode",
			toolName:   regexAlias,
			output:     provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"type":"tool_search_tool_result_error"}`)},
			wireName:   "tool_search_tool_regex",
			wireResult: `{"type":"tool_search_tool_result_error","error_code":"unavailable"}`,
		},
		{
			name:       "typed JSON error",
			toolName:   bm25Alias,
			output:     provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"type":"tool_search_tool_result_error","errorCode":"execution_time_exceeded"}`)},
			wireName:   "tool_search_tool_bm25",
			wireResult: `{"type":"tool_search_tool_result_error","error_code":"execution_time_exceeded"}`,
		},
		{
			name:       "typed JSON error without errorCode",
			toolName:   regexAlias,
			output:     provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"type":"tool_search_tool_result_error"}`)},
			wireName:   "tool_search_tool_regex",
			wireResult: `{"type":"tool_search_tool_result_error","error_code":"unavailable"}`,
		},
		{
			name:       "error-json non-JSON string",
			toolName:   bm25Alias,
			output:     provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`"boom"`)},
			wireName:   "tool_search_tool_bm25",
			wireResult: `{"type":"tool_search_tool_result_error","error_code":"unavailable"}`,
		},
		{
			name:       "error-json null errorCode",
			toolName:   regexAlias,
			output:     provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"type":"tool_search_tool_result_error","errorCode":null}`)},
			wireName:   "tool_search_tool_regex",
			wireResult: `{"type":"tool_search_tool_result_error","error_code":"unavailable"}`,
		},
		{
			name:       "error-json non-string errorCode",
			toolName:   bm25Alias,
			output:     provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"errorCode":5}`)},
			wireName:   "tool_search_tool_bm25",
			wireResult: `{"type":"tool_search_tool_result_error","error_code":"unavailable"}`,
		},
		{
			name:       "error with cache marker",
			toolName:   bm25Alias,
			output:     provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"type":"tool_search_tool_result_error","errorCode":"invalid_tool_input"}`)},
			cacheMark:  true,
			wireName:   "tool_search_tool_bm25",
			wireResult: `{"type":"tool_search_tool_result_error","error_code":"invalid_tool_input"}`,
		},
		{
			name:       "successful empty references",
			toolName:   regexAlias,
			output:     provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`[]`)},
			wireName:   "tool_search_tool_regex",
			wireResult: `{"type":"tool_search_tool_search_result","tool_references":[]}`,
		},
		{
			name:       "successful references with cache marker",
			toolName:   bm25Alias,
			output:     provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`[{"type":"tool_reference","toolName":"get_weather"}]`)},
			cacheMark:  true,
			wireName:   "tool_search_tool_bm25",
			wireResult: `{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","tool_name":"get_weather"}]}`,
		},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/streaming=%t", tc.name, streaming), func(t *testing.T) {
				call := provider.ToolCallPart("srvtoolu_1", tc.toolName, json.RawMessage(`{"query":"weather"}`))
				call.ProviderExecuted = true
				result := provider.ToolResultPart("srvtoolu_1", tc.toolName, &tc.output)
				result.ProviderExecuted = true
				cacheControl := ""
				if tc.cacheMark {
					result.ProviderOptions = makeProviderOpts(`{"cacheControl":{"type":"ephemeral"}}`)
					cacheControl = `,"cache_control":{"type":"ephemeral"}`
				}
				opts := provider.CallOptions{
					MaxOutputTokens: new(64),
					Tools:           tools,
					Prompt:          []provider.Message{provider.UserText("find a weather tool"), provider.NewAssistantMessage(call, result)},
				}

				_, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, streaming)
				require.NoError(t, err)
				assert.Empty(t, warnings)

				body := captureMessagesRequest(t, opts, streaming)

				var messages []struct {
					Content json.RawMessage `json:"content"`
				}
				require.NoError(t, json.Unmarshal(body["messages"], &messages))
				require.Len(t, messages, 2)
				assert.JSONEq(t, `[
					{"type":"server_tool_use","id":"srvtoolu_1","name":"`+tc.wireName+`","input":{"query":"weather"}},
					{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_1","content":`+tc.wireResult+cacheControl+`}
				]`, string(messages[1].Content))
			})
		}
	}
}

func TestToolSearchResult_UntypedJSONObjectWarns(t *testing.T) {
	result := provider.ToolResultPart("srvtoolu_1", "tool_search_tool_regex", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"type":"other","errorCode":"invalid_tool_input"}`)})
	result.ProviderExecuted = true
	params, _, warnings, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(result)}}, false)
	require.NoError(t, err)
	assert.Empty(t, params.Messages)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Message, "failed to unmarshal tool_search result")
}

func captureMessagesRequest(t *testing.T, opts provider.CallOptions, streaming bool) map[string]json.RawMessage {
	t.Helper()
	requests := make(chan map[string]json.RawMessage, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"message":"captured","type":"invalid_request_error"}}`)
	}))
	defer server.Close()
	model := New("test", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))

	var err error
	if streaming {
		_, err = model.DoStream(context.Background(), opts)
	} else {
		_, err = model.DoGenerate(context.Background(), opts)
	}
	var apiError *provider.APICallError
	require.ErrorAs(t, err, &apiError)
	require.Len(t, requests, 1)
	return <-requests
}
