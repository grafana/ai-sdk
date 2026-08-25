package agentobservability

import (
	"encoding/json"
	"testing"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessagesToAgento11y_SystemMessageFolding(t *testing.T) {
	prompt := []provider.Message{
		provider.NewSystemMessage("you are a helpful assistant"),
		provider.NewSystemMessage("be concise"),
		provider.UserText("hi"),
	}
	system, msgs := messagesToAgento11y(prompt)
	assert.Equal(t, "you are a helpful assistant\n\nbe concise", system)
	require.Len(t, msgs, 1)
	assert.Equal(t, agento11y.RoleUser, msgs[0].Role)
}

func TestMessagesToAgento11y_EmptyPrompt(t *testing.T) {
	system, msgs := messagesToAgento11y(nil)
	assert.Empty(t, system)
	assert.Nil(t, msgs)
}

func TestMessagesToAgento11y_AssistantWithReasoning(t *testing.T) {
	prompt := []provider.Message{
		provider.NewAssistantMessage(
			provider.ReasoningPart("thinking…"),
			provider.TextPart("hello"),
		),
	}
	_, msgs := messagesToAgento11y(prompt)
	require.Len(t, msgs, 1)
	assert.Equal(t, agento11y.RoleAssistant, msgs[0].Role)
	require.Len(t, msgs[0].Parts, 2)
	assert.Equal(t, agento11y.PartKindThinking, msgs[0].Parts[0].Kind)
	assert.Equal(t, "thinking…", msgs[0].Parts[0].Thinking)
	assert.Equal(t, "thinking", msgs[0].Parts[0].Metadata.ProviderType)
	assert.Equal(t, agento11y.PartKindText, msgs[0].Parts[1].Kind)
}

func TestMessagesToAgento11y_SkipsEmptyReasoning(t *testing.T) {
	prompt := []provider.Message{provider.NewAssistantMessage(
		provider.ReasoningPart(""),
		provider.TextPart("answer"),
	)}

	_, msgs := messagesToAgento11y(prompt)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Parts, 1)
	assert.Equal(t, agento11y.PartKindText, msgs[0].Parts[0].Kind)
}

func TestMessagesToAgento11y_ToolResultSplitting(t *testing.T) {
	// A user message that mixes a text part and a tool_result part should be
	// split: the text part lands in a RoleUser agento11y.Message; the tool_result
	// part lands in a separate RoleTool agento11y.Message. This matches the
	// upstream Anthropic mapper's behavior.
	toolResultJSON := json.RawMessage(`{"output":"ok"}`)
	prompt := []provider.Message{
		provider.NewUserMessage(
			provider.TextPart("did it work?"),
			provider.ContentPart{
				Type:       provider.ContentPartTypeToolResult,
				ToolCallID: "tc-1",
				ToolName:   "lookup",
				Output: &provider.ToolResultOutput{
					Type: provider.ToolOutputJSON,
					JSON: toolResultJSON,
				},
			},
		),
	}
	_, msgs := messagesToAgento11y(prompt)
	require.Len(t, msgs, 2, "expected user message + tool message split")
	assert.Equal(t, agento11y.RoleUser, msgs[0].Role)
	require.Len(t, msgs[0].Parts, 1)
	assert.Equal(t, agento11y.PartKindText, msgs[0].Parts[0].Kind)

	assert.Equal(t, agento11y.RoleTool, msgs[1].Role)
	require.Len(t, msgs[1].Parts, 1)
	assert.Equal(t, agento11y.PartKindToolResult, msgs[1].Parts[0].Kind)
	require.NotNil(t, msgs[1].Parts[0].ToolResult)
	assert.Equal(t, "tc-1", msgs[1].Parts[0].ToolResult.ToolCallID)
	assert.JSONEq(t, string(toolResultJSON), string(msgs[1].Parts[0].ToolResult.ContentJSON))
}

func TestMessagesToAgento11y_FileParts(t *testing.T) {
	prompt := []provider.Message{
		provider.NewUserMessage(
			provider.FilePart("image/*", provider.DataContent{URL: "data:image/png;base64,AQID"}),
		),
		provider.NewAssistantMessage(
			provider.ReasoningFilePart("video/mp4", provider.DataContent{Bytes: []byte{1, 2, 3}}),
		),
	}

	_, msgs := messagesToAgento11y(prompt)
	require.Len(t, msgs, 2)

	file := msgs[0].Parts[0]
	assert.Equal(t, agento11y.PartKindMedia, file.Kind)
	require.NotNil(t, file.Media)
	assert.Equal(t, "image", file.Media.Kind)
	assert.Equal(t, "data:image/png;base64,AQID", file.Media.URL)
	assert.Equal(t, "image/png", file.Media.MIMEType)
	assert.Equal(t, "file", file.Metadata.ProviderType)

	reasoningFile := msgs[1].Parts[0]
	assert.Equal(t, agento11y.PartKindMedia, reasoningFile.Kind)
	require.NotNil(t, reasoningFile.Media)
	assert.Equal(t, "video", reasoningFile.Media.Kind)
	assert.Equal(t, "data:video/mp4;base64,AQID", reasoningFile.Media.URL)
	assert.Equal(t, "reasoning_file", reasoningFile.Metadata.ProviderType)
}

func TestMessagesToAgento11y_UnsupportedFileData(t *testing.T) {
	tests := []struct {
		name string
		data provider.DataContent
	}{
		{name: "reference", data: provider.DataContent{Reference: json.RawMessage(`{"openai":"file-1"}`)}},
		{name: "text", data: provider.DataContent{Text: "inline document"}},
		{name: "malformed base64", data: provider.DataContent{Base64: "%%%"}},
		{name: "base64 newline", data: provider.DataContent{Base64: "AQ\nID"}},
		{name: "escaped base64 newline", data: provider.DataContent{URL: "data:image/png;base64,AQ%0AID"}},
		{name: "credential URL", data: provider.DataContent{URL: "https://user:pass@cdn.example.com/image.png"}},
		{name: "non-HTTP URL", data: provider.DataContent{URL: "file:///tmp/image.png"}},
		{name: "malformed data URL header", data: provider.DataContent{URL: "data:image/png;invalid,AQID"}},
		{name: "multiple sources", data: provider.DataContent{Base64: "AQID", URL: "https://cdn.example.com/image.png"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prompt := []provider.Message{
				provider.NewUserMessage(provider.FilePart("image/png", tc.data)),
			}
			_, msgs := messagesToAgento11y(prompt)
			assert.Nil(t, msgs)
		})
	}
}

func TestMessagesToAgento11y_AssistantToolCall(t *testing.T) {
	input := json.RawMessage(`{"q":"weather"}`)
	prompt := []provider.Message{
		provider.NewAssistantMessage(
			provider.ToolCallPart("tc-1", "lookup", input),
		),
	}
	_, msgs := messagesToAgento11y(prompt)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Parts, 1)
	assert.Equal(t, agento11y.PartKindToolCall, msgs[0].Parts[0].Kind)
	require.NotNil(t, msgs[0].Parts[0].ToolCall)
	assert.Equal(t, "tc-1", msgs[0].Parts[0].ToolCall.ID)
	assert.Equal(t, "lookup", msgs[0].Parts[0].ToolCall.Name)
	assert.JSONEq(t, string(input), string(msgs[0].Parts[0].ToolCall.InputJSON))
	assert.Equal(t, "tool_use", msgs[0].Parts[0].Metadata.ProviderType)
}

func TestMessagesToAgento11y_AssistantServerToolCall(t *testing.T) {
	input := json.RawMessage(`{"q":"weather"}`)
	part := provider.ToolCallPart("tc-1", "web_search", input)
	part.ProviderExecuted = true
	prompt := []provider.Message{provider.NewAssistantMessage(part)}
	_, msgs := messagesToAgento11y(prompt)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Parts, 1)
	assert.Equal(t, "server_tool_use", msgs[0].Parts[0].Metadata.ProviderType)
}

func TestToolsToAgento11y(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{}}`)
	tools := []provider.Tool{
		{
			Type:        provider.ToolTypeFunction,
			Name:        "get_weather",
			Description: "Get weather",
			InputSchema: schema,
		},
		{
			Type: provider.ToolTypeProvider,
			Name: "web_search",
			ID:   "web_search_20250305",
		},
		{
			// Empty name should be skipped.
			Type: provider.ToolTypeFunction,
			Name: "",
		},
	}
	out := toolsToAgento11y(tools)
	require.Len(t, out, 2)
	assert.Equal(t, "get_weather", out[0].Name)
	assert.Equal(t, "Get weather", out[0].Description)
	assert.JSONEq(t, string(schema), string(out[0].InputSchema))
	assert.Empty(t, out[0].Type, "function tool has empty Type for byte-equal parity")

	assert.Equal(t, "web_search", out[1].Name)
	assert.Equal(t, "web_search_20250305", out[1].Type)
}

func TestToolsToAgento11y_DeferLoading(t *testing.T) {
	tools := []provider.Tool{
		{
			Type: provider.ToolTypeFunction,
			Name: "deferred_tool",
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{
					Key: "anthropic",
					Raw: json.RawMessage(`{"deferLoading":true}`),
				},
			},
		},
		{
			Type: provider.ToolTypeFunction,
			Name: "eager_tool",
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{
					Key: "anthropic",
					Raw: json.RawMessage(`{"deferLoading":false}`),
				},
			},
		},
		{
			Type: provider.ToolTypeFunction,
			Name: "unset_tool",
		},
	}
	out := toolsToAgento11y(tools)
	require.Len(t, out, 3)
	assert.True(t, out[0].Deferred)
	assert.False(t, out[1].Deferred)
	assert.False(t, out[2].Deferred)
}

func TestControlsFromCallOptions(t *testing.T) {
	maxTok := 1024
	temp := 0.5
	topP := 0.9
	params := provider.CallOptions{
		MaxOutputTokens: &maxTok,
		Temperature:     &temp,
		TopP:            &topP,
		ToolChoice:      &provider.ToolChoice{Type: provider.ToolChoiceAuto},
	}
	ctrl := controlsFromCallOptions(params)
	require.NotNil(t, ctrl.MaxTokens)
	assert.Equal(t, int64(1024), *ctrl.MaxTokens)
	require.NotNil(t, ctrl.Temperature)
	assert.InDelta(t, 0.5, *ctrl.Temperature, 1e-9)
	require.NotNil(t, ctrl.TopP)
	assert.InDelta(t, 0.9, *ctrl.TopP, 1e-9)
	require.NotNil(t, ctrl.ToolChoice)
	assert.Equal(t, "auto", *ctrl.ToolChoice)
}

func TestControlsFromCallOptions_Unset(t *testing.T) {
	ctrl := controlsFromCallOptions(provider.CallOptions{})
	assert.Nil(t, ctrl.MaxTokens)
	assert.Nil(t, ctrl.Temperature)
	assert.Nil(t, ctrl.TopP)
	assert.Nil(t, ctrl.ToolChoice)
}

func TestToolChoiceToAgento11y(t *testing.T) {
	tests := []struct {
		name   string
		choice *provider.ToolChoice
		want   string
	}{
		{"nil", nil, ""},
		{"auto", &provider.ToolChoice{Type: provider.ToolChoiceAuto}, "auto"},
		{"none", &provider.ToolChoice{Type: provider.ToolChoiceNone}, "none"},
		{"required", &provider.ToolChoice{Type: provider.ToolChoiceRequired}, "any"},
		{
			"tool",
			&provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"},
			`{"name":"lookup","type":"tool"}`,
		},
		{"tool with empty name", &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "  "}, ""},
		{"unknown type", &provider.ToolChoice{Type: "weird"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toolChoiceToAgento11y(tc.choice)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMetadataFromProviderOptions_AnthropicThinkingBudget(t *testing.T) {
	// camelCase form (ai-sdk anthropic provider serializes this way).
	params := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{
				Key: "anthropic",
				Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":2048}}`),
			},
		},
	}
	got := metadataFromProviderOptions(params)
	require.NotNil(t, got)
	assert.Equal(t, int64(2048), got[MetadataThinkingBudgetTokens])

	// snake_case form (upstream Anthropic SDK serializes this way).
	params2 := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{
				Key: "anthropic",
				Raw: json.RawMessage(`{"thinking":{"type":"enabled","budget_tokens":4096}}`),
			},
		},
	}
	got2 := metadataFromProviderOptions(params2)
	require.NotNil(t, got2)
	assert.Equal(t, int64(4096), got2[MetadataThinkingBudgetTokens])

	// Disabled / no budget → nil.
	params3 := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{
				Key: "anthropic",
				Raw: json.RawMessage(`{"thinking":{"type":"disabled"}}`),
			},
		},
	}
	assert.Nil(t, metadataFromProviderOptions(params3))

	// No provider options at all.
	assert.Nil(t, metadataFromProviderOptions(provider.CallOptions{}))
}

func TestThinkingEnabledFromAnthropic(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want *bool
	}{
		{"enabled", `{"thinking":{"type":"enabled","budgetTokens":1024}}`, boolPtr(true)},
		{"adaptive", `{"thinking":{"type":"adaptive"}}`, boolPtr(true)},
		{"disabled", `{"thinking":{"type":"disabled"}}`, boolPtr(false)},
		{"unknown", `{"thinking":{"type":"weird"}}`, nil},
		{"no thinking", `{}`, nil},
		{"empty options", `null`, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{
					Key: "anthropic",
					Raw: json.RawMessage(tc.raw),
				},
			}
			got := thinkingEnabledFromAnthropic(opts)
			if tc.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tc.want, *got)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }
