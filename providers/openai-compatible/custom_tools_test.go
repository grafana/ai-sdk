package openaicompatible

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeCustomTools(t *testing.T) {
	tool := provider.Tool{Type: provider.ToolTypeProvider, ID: "openai.custom", Name: "code", Args: map[string]json.RawMessage{"format": json.RawMessage(`{"type":"grammar","grammar":{"syntax":"regex","definition":"[a-z]+"}}`)}}
	call := provider.ToolCallPart("call", "code", json.RawMessage(`"print('hello')"`))
	for _, declare := range []bool{false, true} {
		options := provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(call)}, ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "code"}}
		if declare {
			options.Tools = []provider.Tool{tool}
		} else {
			options.Prompt[0].Content[0].ProviderOptions = provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"customTool":true}`)}}
		}
		m := &model{providerName: "custom.chat", modelID: "test"}
		body, _, err := m.buildRequest(options, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"custom":{"name":"code","input":"print('hello')"}`)
		assert.NotContains(t, string(encoded), `"customTool"`)
		if declare {
			assert.Contains(t, string(encoded), `"tool_choice":{"custom":{"name":"code"},"type":"custom"}`)
			assert.Contains(t, string(encoded), `"grammar":{"syntax":"regex","definition":"[a-z]+"}`)
		}
	}
	response := `{"choices":[{"message":{"tool_calls":[{"id":"call","type":"custom","custom":{"name":"code","input":"print('hello')"}}]},"finish_reason":"tool_calls"}]}`
	result, err := parseGenerateResponse([]byte(response), nil, "custom", "custom", func() string { return "id" }, false)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.JSONEq(t, `"print('hello')"`, string(result.Content[0].Input))
}

func TestNativeCustomToolStream(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call","type":"custom","custom":{"name":"code","input":"print("}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"custom":{"input":"'hello')"}}]},"finish_reason":"tool_calls"}]}

data: [DONE]

`
	parts := make(chan provider.StreamPart, 16)
	m := &model{providerName: "custom.chat"}
	m.runStream(t.Context(), "https://example.test", nil, strings.NewReader(stream), nil, nil, false, "custom", parts, false)
	close(parts)
	var text strings.Builder
	for part := range parts {
		require.NotEqual(t, provider.PartError, part.Type)
		switch part.Type {
		case provider.PartToolInputStart:
			assert.Equal(t, provider.ToolInputEncodingText, part.InputEncoding)
		case provider.PartToolInputDelta:
			text.WriteString(part.Delta)
		case provider.PartToolCall:
			assert.JSONEq(t, `"print('hello')"`, part.Input)
		}
	}
	assert.Equal(t, "print('hello')", text.String())
}

func TestInvalidCustomToolFormat(t *testing.T) {
	for _, format := range []string{`{"type":"grammar"}`, `{"type":"text","grammar":{"syntax":"regex","definition":"x"}}`, `{"type":"grammar","grammar":{"syntax":"unknown","definition":"x"}}`} {
		err := validateCustomTools([]provider.Tool{{Type: provider.ToolTypeProvider, ID: "openai.custom", Name: "code", Args: map[string]json.RawMessage{"format": json.RawMessage(format)}}})
		require.Error(t, err)
	}
}

func TestCustomToolResultMarkerIsLocal(t *testing.T) {
	for _, marker := range []string{`true`, `false`, `"true"`} {
		part := provider.ToolResultPart("c", "custom", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "result"})
		part.ProviderOptions = provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"customTool":` + marker + `}`)}}
		result, err := convertToolMessages([]provider.ContentPart{part})
		if marker == `"true"` {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		raw, err := json.Marshal(result)
		require.NoError(t, err)
		assert.JSONEq(t, `[{"role":"tool","content":"result","tool_call_id":"c"}]`, string(raw))
	}
}
