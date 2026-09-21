package v4

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeFunctionTools_NativeAnthropicRequests(t *testing.T) {
	type capturedRequest struct {
		body  map[string]any
		betas string
	}
	requests := make(chan capturedRequest, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var captured map[string]any
		require.NoError(t, json.Unmarshal(body, &captured))
		requests <- capturedRequest{body: captured, betas: r.Header.Get("anthropic-beta")}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"response","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()
	harness := newRuntimeHarness(t, testLimits())
	model := anthropicprovider.New("test-key", "claude-sonnet-4-6", anthropicprovider.WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
	harness.model.generate = func(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
		result, err := model.DoGenerate(ctx, options)
		assert.NoError(t, err)
		return result, err
	}
	definitions := `"maxOutputTokens":64,"tools":[{"type":"function","name":"weather","inputSchema":{"type":"object","properties":{"city":{"type":"string"}}},"strict":false,"inputExamples":[{"input":{"city":"Rio"}}]}],"toolChoice":{"type":"tool","toolName":"weather"}`
	first := harness.serve(validRequest(`{"prompt":[{"role":"user","content":[{"type":"text","text":"weather"}]}],` + definitions + `}`))
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	captured := <-requests
	native := captured.body
	assert.Contains(t, captured.betas, "advanced-tool-use-2025-11-20")
	tools := native["tools"].([]any)
	require.Len(t, tools, 1)
	tool := tools[0].(map[string]any)
	assert.Equal(t, false, tool["strict"])
	assert.Equal(t, []any{map[string]any{"city": "Rio"}}, tool["input_examples"])
	assert.Equal(t, map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}}, tool["input_schema"])
	assert.Equal(t, "weather", native["tool_choice"].(map[string]any)["name"])
	second := harness.serve(validRequest(`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"weather","input":{"city":"Rio"}}]},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"weather","output":{"type":"text","value":""}}]}],` + definitions + `}`))
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	captured = <-requests
	native = captured.body
	messages := native["messages"].([]any)
	require.Len(t, messages, 2)
	call := messages[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "tool_use", call["type"])
	assert.Equal(t, "call", call["id"])
	assert.Equal(t, map[string]any{"city": "Rio"}, call["input"])
	result := messages[1].(map[string]any)["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "tool_result", result["type"])
	assert.Equal(t, "call", result["tool_use_id"])
	assert.Equal(t, []any{map[string]any{"type": "text", "text": ""}}, result["content"])
	for _, tc := range []struct {
		name                string
		definition          string
		wantInputExamples   bool
		wantAllowedCallers  bool
		wantAdvancedToolUse bool
	}{
		{
			name:       "omitted",
			definition: `"maxOutputTokens":64,"tools":[{"type":"function","name":"weather","inputSchema":{"type":"object"}}],"toolChoice":{"type":"tool","toolName":"weather"}`,
		},
		{
			name:                "explicit empty input examples",
			definition:          `"maxOutputTokens":64,"tools":[{"type":"function","name":"weather","inputSchema":{"type":"object"},"inputExamples":[]}],"toolChoice":{"type":"tool","toolName":"weather"}`,
			wantInputExamples:   true,
			wantAdvancedToolUse: true,
		},
		{
			name:                "explicit empty allowed callers",
			definition:          `"maxOutputTokens":64,"tools":[{"type":"function","name":"weather","inputSchema":{"type":"object"},"providerOptions":{"anthropic":{"allowedCallers":[]}}}],"toolChoice":{"type":"tool","toolName":"weather"}`,
			wantAllowedCallers:  true,
			wantAdvancedToolUse: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := harness.serve(validRequest(`{"prompt":[{"role":"user","content":[{"type":"text","text":"weather"}]}],` + tc.definition + `}`))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			captured := <-requests
			tool := captured.body["tools"].([]any)[0].(map[string]any)
			inputExamples, hasInputExamples := tool["input_examples"]
			allowedCallers, hasAllowedCallers := tool["allowed_callers"]
			assert.Equal(t, tc.wantInputExamples, hasInputExamples)
			assert.Equal(t, tc.wantAllowedCallers, hasAllowedCallers)
			if tc.wantInputExamples {
				assert.Equal(t, []any{}, inputExamples)
			}
			if tc.wantAllowedCallers {
				assert.Equal(t, []any{}, allowedCallers)
			}
			assert.Equal(t, tc.wantAdvancedToolUse, strings.Contains(captured.betas, "advanced-tool-use-2025-11-20"))
		})
	}
}
