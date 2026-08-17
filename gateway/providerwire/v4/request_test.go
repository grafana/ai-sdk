package providerwirev4

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallOptions_StrictCanonicalRoundTrip(t *testing.T) {
	strict := true
	maxTokens := 100
	temperature := 0.3
	topP := 0.9
	topK := 20
	penalty := 0.1
	seed := 42
	reasoning := provider.ReasoningHigh
	options := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("system"),
			provider.NewUserMessage(
				provider.TextPart("hello"),
				provider.FilePart("image/png", provider.DataContent{Base64: "AAEC"}),
			),
			provider.NewAssistantMessage(
				provider.ReasoningPart("thinking"),
				provider.ReasoningFilePart("image/png", provider.DataContent{URL: "https://example.com/reasoning.png"}),
				provider.CustomPart("provider.custom"),
				provider.ToolCallPart("call-1", "search", json.RawMessage(`{"q":"go"}`)),
				provider.ToolResultPart("call-2", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{
					{Type: provider.ToolContentText, Text: "answer"},
					{Type: provider.ToolContentFile, Data: &provider.DataContent{Reference: json.RawMessage(`{"provider":"file-1"}`)}, MediaType: "application/pdf", Filename: "answer.pdf"},
				}}),
			),
			provider.NewToolMessage(provider.ToolApprovalResponsePart("approval-1", false, "denied")),
		},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "search", Description: "search", InputSchema: json.RawMessage(`{"type":"object"}`), InputExamples: []provider.InputExample{{Input: json.RawMessage(`{"q":"go"}`)}}, Strict: &strict},
			{Type: provider.ToolTypeProvider, Name: "web", ID: "provider.web", Args: map[string]json.RawMessage{"limit": json.RawMessage(`2`)}},
		},
		ToolChoice:       &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "search"},
		MaxOutputTokens:  &maxTokens,
		Temperature:      &temperature,
		TopP:             &topP,
		TopK:             &topK,
		PresencePenalty:  &penalty,
		FrequencyPenalty: &penalty,
		StopSequences:    []string{"STOP"},
		ResponseFormat:   &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object"}`), Name: "result", Description: "result"},
		Seed:             &seed,
		Reasoning:        &reasoning,
		IncludeRawChunks: true,
		Headers:          map[string]string{"X-Trace": "trace"},
		ProviderOptions: provider.ProviderOptions{
			"provider": provider.RawProviderOption{Key: "provider", Raw: json.RawMessage(`{"option":true}`)},
		},
	}

	data, err := EncodeCallOptions(options)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"role":"system","content":"system"`)
	assert.Contains(t, string(data), `"output":{"type":"content","value"`)
	decoded, err := decodeCallOptionsJSON(data)
	require.NoError(t, err)
	assert.Equal(t, options, decoded)
}

func TestCallOptions_PinnedCanonicalGolden(t *testing.T) {
	golden := `{
		"prompt":[
			{"role":"system","content":"system"},
			{"role":"assistant","content":[
				{"type":"tool-call","toolCallId":"call","toolName":"search","input":{"q":"go"}},
				{"type":"tool-result","toolCallId":"call","toolName":"search","output":{"type":"json","value":{"ok":true}}}
			]},
			{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"approval","approved":true}]}
		],
		"tools":[{"type":"function","name":"search","inputSchema":{"type":"object"}}],
		"toolChoice":{"type":"auto"},
		"responseFormat":{"type":"json","schema":{"type":"object"}}
	}`
	decoded, err := decodeCallOptionsJSON([]byte(golden))
	require.NoError(t, err)
	encoded, err := EncodeCallOptions(decoded)
	require.NoError(t, err)
	var compact bytes.Buffer
	require.NoError(t, json.Compact(&compact, []byte(golden)))
	assert.Equal(t, compact.String(), string(encoded))
}

func TestCallOptions_PreservesExplicitEmptyOptionalCollections(t *testing.T) {
	options := provider.CallOptions{
		Prompt:        []provider.Message{},
		Tools:         []provider.Tool{},
		StopSequences: []string{},
	}

	encoded, err := EncodeCallOptions(options)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[],"tools":[],"stopSequences":[]}`, string(encoded))

	decoded, err := decodeCallOptionsJSON(encoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.Tools)
	assert.Empty(t, decoded.Tools)
	require.NotNil(t, decoded.StopSequences)
	assert.Empty(t, decoded.StopSequences)

	absent, err := decodeCallOptionsJSON([]byte(`{"prompt":[]}`))
	require.NoError(t, err)
	assert.Nil(t, absent.Tools)
	assert.Nil(t, absent.StopSequences)
}

func TestJSONObjectBoundaries(t *testing.T) {
	invalid := []string{`null`, `1`, `[]`, `"value"`}
	for _, value := range invalid {
		t.Run(value, func(t *testing.T) {
			_, err := decodeCallOptionsJSON([]byte(`{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":` + value + `}]}`))
			require.Error(t, err)
			_, err = EncodeCallOptions(provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool", InputSchema: json.RawMessage(value)}}})
			require.Error(t, err)

			_, err = decodeCallOptionsJSON([]byte(`{"prompt":[],"responseFormat":{"type":"json","schema":` + value + `}}`))
			require.Error(t, err)
			_, err = EncodeCallOptions(provider.CallOptions{ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(value)}})
			require.Error(t, err)

		})
	}

	validCall := provider.CallOptions{
		Tools:          []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool", InputSchema: json.RawMessage(`{}`), InputExamples: []provider.InputExample{{Input: json.RawMessage(`{}`)}}}},
		ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{}`)},
	}
	encoded, err := EncodeCallOptions(validCall)
	require.NoError(t, err)
	_, err = decodeCallOptionsJSON(encoded)
	require.NoError(t, err)
}

func TestCallOptions_StrictRejectionAndUnknownFields(t *testing.T) {
	cases := []struct {
		name string
		wire string
	}{
		{name: "missing prompt", wire: `{}`},
		{name: "legacy system array", wire: `{"prompt":[{"role":"system","content":[{"type":"text","text":"legacy"}]}]}`},
		{name: "unknown role", wire: `{"prompt":[{"role":"future","content":[]}]}`},
		{name: "unknown content", wire: `{"prompt":[{"role":"user","content":[{"type":"future"}]}]}`},
		{name: "wrong role content", wire: `{"prompt":[{"role":"user","content":[{"type":"reasoning","text":"x"}]}]}`},
		{name: "legacy tool output", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"text","text":"legacy"}}]}]}`},
		{name: "legacy tool file data", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"file-data","data":"AAEC","mediaType":"x"}]}}]}]}`},
		{name: "legacy tool file URL", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"file-url","url":"https://example.com","mediaType":"x"}]}}]}]}`},
		{name: "legacy tool file reference", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"file-reference","reference":{"p":"id"},"mediaType":"x"}]}}]}]}`},
		{name: "unknown file data", wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"future"},"mediaType":"x"}]}]}`},
		{name: "unknown tool", wire: `{"prompt":[],"tools":[{"type":"future","name":"tool"}]}`},
		{name: "unknown tool output", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"future"}}]}]}`},
		{name: "unknown tool result content", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"future"}]}}]}]}`},
		{name: "invalid tool choice", wire: `{"prompt":[],"toolChoice":{"type":"tool"}}`},
		{name: "provider option scalar", wire: `{"prompt":[],"providerOptions":{"provider":1}}`},
		{name: "unknown call options field", wire: `{"prompt":[],"future":true}`},
		{name: "unknown message field", wire: `{"prompt":[{"role":"user","content":[],"future":true}]}`},
		{name: "unknown content field", wire: `{"prompt":[{"role":"user","content":[{"type":"text","text":"x","future":true}]}]}`},
		{name: "unknown file data field", wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"data","data":"","future":true},"mediaType":"x"}]}]}`},
		{name: "unknown tool field", wire: `{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":{},"future":true}]}`},
		{name: "unknown tool input example field", wire: `{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":{},"inputExamples":[{"input":{},"future":true}]}]}`},
		{name: "unknown tool choice field", wire: `{"prompt":[],"toolChoice":{"type":"none","future":true}}`},
		{name: "unknown response format field", wire: `{"prompt":[],"responseFormat":{"type":"text","future":true}}`},
		{name: "unknown tool output field", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"text","value":"ok","future":true}}]}]}`},
		{name: "unknown tool result content field", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"text","text":"ok","future":true}]}}]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeCallOptionsJSON([]byte(tc.wire))
			require.Error(t, err)
		})
	}

	inactive := `{"prompt":[{"role":"user","content":[{"type":"text","text":"x","data":false},{"type":"file","data":{"type":"data","data":"","url":false,"reference":false,"text":false},"mediaType":"x"}]}],"tools":[{"type":"function","name":"tool","inputSchema":{},"id":false,"args":false}],"toolChoice":{"type":"none","toolName":false},"responseFormat":{"type":"text","schema":false,"name":false,"description":false},"providerOptions":{"provider":{"future":true}}}`
	decoded, err := decodeCallOptionsJSON([]byte(inactive))
	require.NoError(t, err)
	assert.Empty(t, decoded.Prompt[0].Content[0].Data)
	assert.Empty(t, decoded.Tools[0].Args)
	assert.Empty(t, decoded.ToolChoice.ToolName)
	assert.Empty(t, decoded.ResponseFormat.Schema)
	assert.Contains(t, decoded.ProviderOptions, "provider")

	for _, options := range []string{`null`, `false`, `[]`, `{"provider":{"future":true}}`} {
		providerToolInactive := `{"prompt":[],"tools":[{"type":"provider","id":"p.tool","name":"tool","args":{},"description":false,"inputSchema":false,"inputExamples":false,"strict":"invalid","providerOptions":` + options + `}]}`
		decoded, err = decodeCallOptionsJSON([]byte(providerToolInactive))
		require.NoError(t, err)
		require.Len(t, decoded.Tools, 1)
		assert.Empty(t, decoded.Tools[0].ProviderOptions)
	}
}

func TestCallOptions_ExplicitExtensionBoundariesRemainOpen(t *testing.T) {
	wire := `{
		"prompt":[
			{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"future.provider":"file-id"}},"mediaType":"application/octet-stream"}]},
			{"role":"assistant","content":[
				{"type":"tool-call","toolCallId":"call","toolName":"tool","input":{"future":{"nested":true}}},
				{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"json","value":{"future":[1,null,true]}}}
			]}
		],
		"tools":[
			{"type":"function","name":"function","inputSchema":{"futureKeyword":{"enabled":true}},"inputExamples":[{"input":{"futureExample":true}}]},
			{"type":"provider","name":"provider","id":"provider.tool","args":{"futureArgument":{"nested":true}}}
		],
		"headers":{"x-future-header":"value"},
		"providerOptions":{"future-provider":{"futureOption":{"nested":true}}}
	}`
	decoded, err := decodeCallOptionsJSON([]byte(wire))
	require.NoError(t, err)
	encoded, err := EncodeCallOptions(decoded)
	require.NoError(t, err)
	assert.JSONEq(t, wire, string(encoded))
}

func TestCallOptions_EncodingRejectsInvalidValues(t *testing.T) {
	cases := []provider.CallOptions{
		{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", provider.DataContent{Bytes: []byte{1}, Base64: "AQ=="}))}},
		{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ToolCallPart("call", "tool", json.RawMessage(`{`)))}},
		{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool"}}},
		{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool}},
		{Tools: []provider.Tool{{Type: provider.ToolTypeProvider, Name: "tool", ID: "p.tool", ProviderOptions: provider.ProviderOptions{"p": provider.RawProviderOption{Key: "p", Raw: json.RawMessage(`{}`)}}}}},
		{ProviderOptions: provider.ProviderOptions{"p": provider.RawProviderOption{Key: "p", Raw: json.RawMessage(`1`)}}},
	}
	for _, options := range cases {
		_, err := EncodeCallOptions(options)
		require.Error(t, err)
	}

	invalidExample := provider.CallOptions{Tools: []provider.Tool{{
		Type: provider.ToolTypeFunction, Name: "tool", InputSchema: json.RawMessage(`{}`),
		InputExamples: []provider.InputExample{{Input: json.RawMessage(`1`)}},
	}}}
	_, err := EncodeCallOptions(invalidExample)
	require.Error(t, err)
}

func TestFlatUnionEncodingUsesDiscriminator(t *testing.T) {
	options := provider.CallOptions{
		Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{
			Type: provider.ContentPartTypeText, Text: "text", Data: &provider.DataContent{Base64: "ignored"},
		})},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeFunction, Name: "tool", InputSchema: json.RawMessage(`{}`),
			ID: "provider.ignored", Args: map[string]json.RawMessage{"ignored": json.RawMessage(`true`)},
		}},
		ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatText, Schema: json.RawMessage(`false`)},
	}
	encoded, err := EncodeCallOptions(options)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[{"role":"user","content":[{"type":"text","text":"text"}]}],"tools":[{"type":"function","name":"tool","inputSchema":{}}],"responseFormat":{"type":"text"}}`, string(encoded))

	_, err = EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(
		provider.FilePart("text/plain", provider.DataContent{Bytes: []byte{}, URL: "https://example.com"}),
	)}})
	require.Error(t, err)
}

func TestStrictCodec_RejectsTypedNullAndPrivateFields(t *testing.T) {
	requests := []string{
		`{"prompt":null}`, `{"prompt":[],"abortSignal":null}`, `{"prompt":[],"stopSequences":[null]}`, `{"prompt":[],"headers":{"x":null}}`,
		`{"prompt":[{"role":"user","content":[{"type":"text","text":null}]}]}`,
		`{"prompt":[{"role":"user","content":[{"type":"text","text":"x","sourceType":null}]}]}`,
		`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"p":null}},"mediaType":"x"}]}]}`,
	}
	for i, wire := range requests {
		t.Run(fmt.Sprintf("request-%d", i), func(t *testing.T) {
			_, err := decodeCallOptionsJSON([]byte(wire))
			require.Error(t, err)
		})
	}

}

func TestStrictCodec_AllowsNullOnlyForOpaqueNullableValues(t *testing.T) {
	request := `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"tool","input":null}]},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"json","value":null}}]}]}`
	decoded, err := decodeCallOptionsJSON([]byte(request))
	require.NoError(t, err)
	assert.Equal(t, "null", string(decoded.Prompt[0].Content[0].Input))
	assert.Equal(t, "null", string(decoded.Prompt[1].Content[0].Output.JSON))
}

func TestLiteralToolChoiceAndResponseFormatGoldens(t *testing.T) {
	cases := []struct {
		name    string
		wire    string
		options provider.CallOptions
	}{
		{name: "tool choice none", wire: `{"prompt":[],"toolChoice":{"type":"none"}}`, options: provider.CallOptions{Prompt: []provider.Message{}, ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceNone}}},
		{name: "tool choice required", wire: `{"prompt":[],"toolChoice":{"type":"required"}}`, options: provider.CallOptions{Prompt: []provider.Message{}, ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceRequired}}},
		{name: "named tool choice", wire: `{"prompt":[],"toolChoice":{"type":"tool","toolName":"search"}}`, options: provider.CallOptions{Prompt: []provider.Message{}, ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "search"}}},
		{name: "text response format", wire: `{"prompt":[],"responseFormat":{"type":"text"}}`, options: provider.CallOptions{Prompt: []provider.Message{}, ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatText}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := EncodeCallOptions(tc.options)
			require.NoError(t, err)
			assert.JSONEq(t, tc.wire, string(encoded))
			decoded, err := decodeCallOptionsJSON([]byte(tc.wire))
			require.NoError(t, err)
			assert.Equal(t, tc.options, decoded)
		})
	}
}

func TestCallOptions_ZeroScalarSettingsAndReasoningLiterals(t *testing.T) {
	zeroInt := 0
	zeroFloat := 0.0
	options := provider.CallOptions{
		Prompt:           []provider.Message{},
		MaxOutputTokens:  &zeroInt,
		Temperature:      &zeroFloat,
		TopP:             &zeroFloat,
		TopK:             &zeroInt,
		PresencePenalty:  &zeroFloat,
		FrequencyPenalty: &zeroFloat,
		Seed:             &zeroInt,
	}
	encoded, err := EncodeCallOptions(options)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[],"maxOutputTokens":0,"temperature":0,"topP":0,"topK":0,"presencePenalty":0,"frequencyPenalty":0,"seed":0}`, string(encoded))
	decoded, err := decodeCallOptionsJSON(encoded)
	require.NoError(t, err)
	assert.Equal(t, options, decoded)

	for _, value := range []provider.ReasoningEffort{
		provider.ReasoningProviderDefault,
		provider.ReasoningNone,
		provider.ReasoningMinimal,
		provider.ReasoningLow,
		provider.ReasoningMedium,
		provider.ReasoningHigh,
		provider.ReasoningXHigh,
	} {
		t.Run(string(value), func(t *testing.T) {
			options := provider.CallOptions{Prompt: []provider.Message{}, Reasoning: &value}
			encoded, err := EncodeCallOptions(options)
			require.NoError(t, err)
			decoded, err := decodeCallOptionsJSON(encoded)
			require.NoError(t, err)
			assert.Equal(t, options, decoded)
		})
	}
}

func TestCallOptions_InvalidLiterals(t *testing.T) {
	for _, wire := range []string{
		`{"prompt":[],"toolChoice":{"type":"future"}}`,
		`{"prompt":[],"responseFormat":{"type":"future"}}`,
		`{"prompt":[],"reasoning":"future"}`,
	} {
		_, err := decodeCallOptionsJSON([]byte(wire))
		require.Error(t, err)
	}
	invalidReasoning := provider.ReasoningEffort("future")
	_, err := EncodeCallOptions(provider.CallOptions{Reasoning: &invalidReasoning})
	require.Error(t, err)
}
