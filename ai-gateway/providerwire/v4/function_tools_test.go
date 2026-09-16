package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeUnaryFunctionTools(t *testing.T) {
	body := `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"weather","input":{"city":"Rio"}}]},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"weather","output":{"type":"json","value":null}}]}],"tools":[{"type":"function","name":"weather","description":"","inputSchema":{"type":"object"},"strict":false,"inputExamples":[{"input":{"city":"Rio"}}],"providerOptions":{"anthropic":{"cacheControl":{"type":"ephemeral"}}}}],"toolChoice":{"type":"tool","toolName":"weather"}}`
	harness := newRuntimeHarness(t, testLimits())
	response := harness.serve(validRequest(body))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	options := harness.model.receivedOptions()
	require.Len(t, options.Tools, 1)
	require.NotNil(t, options.Tools[0].Strict)
	assert.False(t, *options.Tools[0].Strict)
	assert.JSONEq(t, `{"type":"object"}`, string(options.Tools[0].InputSchema))
	require.Len(t, options.Tools[0].InputExamples, 1)
	assert.JSONEq(t, `{"city":"Rio"}`, string(options.Tools[0].InputExamples[0].Input))
	assert.Equal(t, provider.ToolChoiceTool, options.ToolChoice.Type)
	assert.JSONEq(t, `{"city":"Rio"}`, string(options.Prompt[0].Content[0].Input))
	assert.Equal(t, "null", string(options.Prompt[1].Content[0].Output.JSON))
}

func TestUnaryFunctionOutput(t *testing.T) {
	result := validGenerateResult()
	result.Content = append(result.Content, provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "weather", Input: json.RawMessage(`{"city":"Rio"}`), ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"hidden"}`)}})
	mapped, err := mapUnarySuccess(result, 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	assert.Contains(t, string(body), `"input":"{\"city\":\"Rio\"}"`)
	assert.NotContains(t, string(body), "hidden")
	for _, marker := range []string{"providerExecuted", "dynamic"} {
		t.Run(marker, func(t *testing.T) {
			copyResult := *result
			copyResult.Content = append([]provider.GenerateContentPart(nil), result.Content...)
			if marker == "providerExecuted" {
				copyResult.Content[1].ProviderExecuted = true
			} else {
				yes := true
				copyResult.Content[1].Dynamic = &yes
			}
			_, err := mapUnarySuccess(&copyResult, 1<<20)
			require.Error(t, err)
		})
	}
}

func TestRuntimeFunctionTools_Boundaries(t *testing.T) {
	for _, body := range []string{
		`{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{},"args":{}}]}`,
		`{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{},"providerOptions":{"gateway":{}}}]}`,
		`{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{},"providerOptions":{"grafana":{"secret":"private"}}}]}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"a","toolName":"f","input":{},"providerExecuted":true}]}]}`,
		`{"prompt":[{"role":"user","content":[{"type":"tool-call","toolCallId":"a","toolName":"f","input":{}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"a","toolName":"f","output":{"type":"text"}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"a","toolName":"f","output":{"type":"text","value":"","providerOptions":{"anthropic":{"private":true}}}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"a","toolName":"f","output":{"type":"execution-denied","reason":""}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"a","toolName":"f","output":{"type":"content","value":[{"type":"text","text":"","providerOptions":{"p":{"x":1}}}]}}]}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(body))
			require.Equal(t, http.StatusBadRequest, response.Code)
			assert.Zero(t, harness.model.callCount())
			assert.NotContains(t, response.Body.String(), "private")
		})
	}
	for _, body := range []string{
		`{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{}}]}`,
		`{"prompt":[],"toolChoice":{"type":"none"}}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"a","toolName":"f","input":{}}]}]}`,
	} {
		harness := newRuntimeHarness(t, testLimits())
		request := validRequest(body)
		request.Header.Set(HeaderStreaming, "true")
		response := harness.serve(request)
		require.Equal(t, http.StatusBadRequest, response.Code)
		assert.Zero(t, harness.model.callCount())
	}
}

func TestRuntimeFunctionTools_SelectedEmptyArms(t *testing.T) {
	for _, output := range []string{
		`{"type":"text","value":""}`, `{"type":"error-text","value":""}`,
		`{"type":"json","value":null}`, `{"type":"error-json","value":null}`,
		`{"type":"content","value":[]}`, `{"type":"content","value":[{"type":"text","text":""}]}`,
	} {
		harness := newRuntimeHarness(t, testLimits())
		body := `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"a","toolName":"f","output":` + output + `}]}]}`
		response := harness.serve(validRequest(body))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		mapped, err := json.Marshal(harness.model.receivedOptions().Prompt[0].Content[0].Output)
		require.NoError(t, err)
		assert.JSONEq(t, output, string(mapped))
	}
}

func TestRuntimeUnaryFunctionOutput_RejectsEnabledMarkersBeforeSuccess(t *testing.T) {
	for _, marker := range []string{"providerExecuted", "dynamic", "preliminary"} {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			result := validGenerateResult()
			part := provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "private-call", ToolName: "private-tool", Input: json.RawMessage("{}")}
			yes := true
			switch marker {
			case "providerExecuted":
				part.ProviderExecuted = true
			case "dynamic":
				part.Dynamic = &yes
			case "preliminary":
				part.Preliminary = &yes
			}
			result.Content = append(result.Content, part)
			return result, nil
		}
		response := harness.serve(validRequest(`{"prompt":[]}`))
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, string(canonicalInternalError), response.Body.String())
	}
}

func TestUnaryFunctionOutput_CompleteBounds(t *testing.T) {
	result := validGenerateResult()
	result.Content = []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "a", ToolName: "f", Input: json.RawMessage(strings.Repeat("\x00", 32))}}
	mapped, err := mapUnarySuccess(result, 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	_, ok = encodeUnarySuccess(mapped, int64(len(body)))
	assert.True(t, ok)
	_, ok = encodeUnarySuccess(mapped, int64(len(body)-1))
	assert.False(t, ok)
	for _, field := range []string{"id", "name", "input"} {
		copyResult := *result
		copyResult.Content = append([]provider.GenerateContentPart(nil), result.Content...)
		switch field {
		case "id":
			copyResult.Content[0].ToolCallID = strings.Repeat("x", 2048)
		case "name":
			copyResult.Content[0].ToolName = strings.Repeat("x", 2048)
		case "input":
			copyResult.Content[0].Input = json.RawMessage(strings.Repeat("x", 2048))
		}
		_, err := mapUnarySuccess(&copyResult, 1024)
		assert.Error(t, err)
	}
}
