package providerwire

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrFloat(f float64) *float64 { return &f }
func ptrEffort(e provider.ReasoningEffort) *provider.ReasoningEffort {
	return &e
}

// TestDecodeCallOptions_UpstreamGatewayBody decodes an actual request body as
// produced by the upstream Vercel AI SDK @ai-sdk/gateway client (system content
// as a string, user content as parts). It proves the hosted provider wire
// accepts an upstream gateway client without a compatibility shim. See openspec
// change provider-wire-upstream-decode-compat.
func TestDecodeCallOptions_UpstreamGatewayBody(t *testing.T) {
	// Captured from poc/gateway-interop: @ai-sdk/gateway streamText with a
	// system + user prompt.
	body := []byte(`{"toolChoice":{"type":"auto"},"prompt":[` +
		`{"role":"system","content":"You are a helpful assistant."},` +
		`{"role":"user","content":[{"type":"text","text":"Hello with a system prompt"}]}` +
		`],"includeRawChunks":false}`)

	opts, err := DecodeCallOptions(body)
	require.NoError(t, err)
	require.Len(t, opts.Prompt, 2)
	require.NotNil(t, opts.ToolChoice)

	assert.Empty(t, opts.Tools)
	assert.Equal(t, provider.ToolChoiceAuto, opts.ToolChoice.Type)
	assert.Equal(t, provider.RoleSystem, opts.Prompt[0].Role)
	assert.Equal(t, []provider.ContentPart{
		{Type: provider.ContentPartTypeText, Text: "You are a helpful assistant."},
	}, opts.Prompt[0].Content)

	assert.Equal(t, provider.RoleUser, opts.Prompt[1].Role)
	assert.Equal(t, []provider.ContentPart{
		{Type: provider.ContentPartTypeText, Text: "Hello with a system prompt"},
	}, opts.Prompt[1].Content)
}

// TestEncodeDecodeCallOptions_FullRoundTrip asserts every notable
// CallOptions field round-trips losslessly through wire encoding.
func TestEncodeDecodeCallOptions_FullRoundTrip(t *testing.T) {
	full := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("be helpful"),
			provider.NewUserMessage(
				provider.TextPart("describe"),
				provider.FilePart("image/png", provider.DataContent{URL: "https://example.com/x.png"}),
			),
			provider.NewAssistantMessage(
				provider.ReasoningPart("thinking"),
				provider.ToolCallPart("tc_1", "search", json.RawMessage(`{"q":"go"}`)),
			),
			provider.NewToolMessage(
				provider.ContentPart{
					Type: provider.ContentPartTypeToolResult, ToolCallID: "tc_1", ToolName: "search",
					Output: &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"},
				},
			),
		},
		Tools: []provider.Tool{
			{
				Type:        provider.ToolTypeFunction,
				Name:        "search",
				Description: "Searches the web",
				InputSchema: json.RawMessage(`{"type":"object"}`),
				InputExamples: []provider.InputExample{
					{Input: json.RawMessage(`{"q":"hello"}`)},
				},
				Strict: ptrBool(false),
				ProviderOptions: provider.ProviderOptions{
					"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"cache":"ephemeral"}`)},
				},
			},
			{
				Type: provider.ToolTypeProvider,
				Name: "web_search",
				ID:   "anthropic.web_search_20250305",
				Args: map[string]json.RawMessage{
					"maxUses": json.RawMessage(`5`),
				},
			},
		},
		ToolChoice:       &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "search"},
		MaxOutputTokens:  ptrInt(1024),
		Temperature:      ptrFloat(0.7),
		TopP:             ptrFloat(0.95),
		TopK:             ptrInt(40),
		PresencePenalty:  ptrFloat(0.1),
		FrequencyPenalty: ptrFloat(0.2),
		StopSequences:    []string{"END", "\n\n"},
		ResponseFormat:   &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object"}`), Name: "result"},
		Seed:             ptrInt(42),
		Reasoning:        ptrEffort(provider.ReasoningHigh),
		IncludeRawChunks: true,
		Headers:          map[string]string{"X-Trace-ID": "abc"},
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"budget":1024}}`)},
		},
	}

	data, err := EncodeCallOptions(full)
	require.NoError(t, err)

	got, err := DecodeCallOptions(data)
	require.NoError(t, err)
	assert.Equal(t, full, got)
}

// TestEncodeDecodeCallOptions_PerField creates one assertion per notable
// CallOptions field to keep coverage explicit and easy to extend.
func TestEncodeDecodeCallOptions_PerField(t *testing.T) {
	cases := []struct {
		name string
		opts provider.CallOptions
	}{
		{"Prompt", provider.CallOptions{Prompt: []provider.Message{provider.NewSystemMessage("hi")}}},
		{"Tools", provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "t"}}}},
		{"ToolChoice", provider.CallOptions{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceAuto}}},
		{"MaxOutputTokens", provider.CallOptions{MaxOutputTokens: ptrInt(100)}},
		{"Temperature", provider.CallOptions{Temperature: ptrFloat(0.5)}},
		{"TopP", provider.CallOptions{TopP: ptrFloat(0.9)}},
		{"TopK", provider.CallOptions{TopK: ptrInt(50)}},
		{"PresencePenalty", provider.CallOptions{PresencePenalty: ptrFloat(0.5)}},
		{"FrequencyPenalty", provider.CallOptions{FrequencyPenalty: ptrFloat(0.5)}},
		{"StopSequences", provider.CallOptions{StopSequences: []string{"END"}}},
		{"ResponseFormat", provider.CallOptions{ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatText}}},
		{"Seed", provider.CallOptions{Seed: ptrInt(7)}},
		{"Reasoning", provider.CallOptions{Reasoning: ptrEffort(provider.ReasoningMedium)}},
		{"IncludeRawChunks", provider.CallOptions{IncludeRawChunks: true}},
		{"Headers", provider.CallOptions{Headers: map[string]string{"x": "y"}}},
		{"ProviderOptions", provider.CallOptions{ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"k":"v"}`)},
		}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := EncodeCallOptions(tc.opts)
			require.NoError(t, err)
			got, err := DecodeCallOptions(data)
			require.NoError(t, err)
			assert.Equal(t, tc.opts, got)
		})
	}
}

// TestContentPart_AllTypes_RoundTrip asserts every ContentPartType survives
// encode + decode through CallOptions.Prompt.
func TestContentPart_AllTypes_RoundTrip(t *testing.T) {
	all := provider.NewAssistantMessage(
		provider.TextPart("hi"),
		provider.FilePart("image/png", provider.DataContent{URL: "u"}),
		provider.ReasoningPart("r"),
		provider.ReasoningFilePart("image/png", provider.DataContent{Base64: "AAEC"}),
		provider.SourcePart(provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "src_1", URL: "https://example.com", Title: "Example"}),
		provider.ToolCallPart("tc", "t", json.RawMessage(`{}`)),
		provider.ToolResultPart("tc", "t", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}),
		provider.CustomPart("x.y"),
		provider.ToolApprovalRequestPart("apr", "tc", false),
	)
	tool := provider.NewToolMessage(
		provider.ToolApprovalResponsePart("apr", true, ""),
	)
	opts := provider.CallOptions{Prompt: []provider.Message{all, tool}}

	data, err := EncodeCallOptions(opts)
	require.NoError(t, err)
	got, err := DecodeCallOptions(data)
	require.NoError(t, err)
	assert.Equal(t, opts, got)
}

func TestTool_AllVariants_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		tool provider.Tool
	}{
		{name: "function strict absent", tool: provider.Tool{Type: provider.ToolTypeFunction, Name: "fn", InputSchema: json.RawMessage(`{}`)}},
		{name: "function strict true", tool: provider.Tool{Type: provider.ToolTypeFunction, Name: "fn", InputSchema: json.RawMessage(`{}`), Strict: ptrBool(true)}},
		{name: "function strict false", tool: provider.Tool{Type: provider.ToolTypeFunction, Name: "fn", InputSchema: json.RawMessage(`{}`), Strict: ptrBool(false)}},
		{name: "provider", tool: provider.Tool{Type: provider.ToolTypeProvider, Name: "pv", ID: "anthropic.web_search", Args: map[string]json.RawMessage{"k": json.RawMessage(`1`)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := provider.CallOptions{Tools: []provider.Tool{tc.tool}}
			data, err := EncodeCallOptions(opts)
			require.NoError(t, err)
			got, err := DecodeCallOptions(data)
			require.NoError(t, err)
			assert.Equal(t, opts, got)
		})
	}
}
