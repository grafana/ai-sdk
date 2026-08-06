package providerwirev4

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

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
	assert.JSONEq(t, golden, string(encoded))
}

func TestCallOptions_EmptyInlineTextFileDataRoundTrips(t *testing.T) {
	wires := []string{
		`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":""},"mediaType":"text/plain"}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"file","data":{"type":"text","text":""},"mediaType":"text/plain"}]}}]}]}`,
	}
	for _, wire := range wires {
		decoded, err := decodeCallOptionsJSON([]byte(wire))
		require.NoError(t, err)
		encoded, err := EncodeCallOptions(decoded)
		require.NoError(t, err)
		assert.JSONEq(t, wire, string(encoded))
	}
}

func TestCallOptions_PinnedContentAndFileDataGoldens(t *testing.T) {
	cases := []struct {
		name    string
		options provider.CallOptions
		wire    string
	}{
		{name: "text", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.TextPart("hello"))}}, wire: `{"prompt":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`},
		{name: "reasoning", options: provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ReasoningPart("thinking"))}}, wire: `{"prompt":[{"role":"assistant","content":[{"type":"reasoning","text":"thinking"}]}]}`},
		{name: "custom", options: provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.CustomPart("provider.custom"))}}, wire: `{"prompt":[{"role":"assistant","content":[{"type":"custom","kind":"provider.custom"}]}]}`},
		{name: "file data", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", provider.DataContent{Base64: "AAEC"}))}}, wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"data","data":"AAEC"},"mediaType":"image/png"}]}]}`},
		{name: "file URL", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", provider.DataContent{URL: "https://example.com/file.png"}))}}, wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"url","url":"https://example.com/file.png"},"mediaType":"image/png"}]}]}`},
		{name: "file reference", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("application/pdf", provider.DataContent{Reference: json.RawMessage(`{"provider":"file-1"}`)}))}}, wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"provider":"file-1"}},"mediaType":"application/pdf"}]}]}`},
		{name: "file text", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("text/plain", provider.DataContent{Text: "inline"}))}}, wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":"inline"},"mediaType":"text/plain"}]}]}`},
		{name: "reasoning file", options: provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ReasoningFilePart("image/png", provider.DataContent{URL: "https://example.com/reasoning.png"}))}}, wire: `{"prompt":[{"role":"assistant","content":[{"type":"reasoning-file","data":{"type":"url","url":"https://example.com/reasoning.png"},"mediaType":"image/png"}]}]}`},
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

func TestCallOptions_GatewayNamespaceIsRemovedOrRejected(t *testing.T) {
	for _, wire := range []string{
		`{"prompt":[]}`,
		`{"prompt":[],"providerOptions":{"gateway":{}}}`,
		`{"prompt":[],"providerOptions":{"provider":{"keep":true},"gateway":{}}}`,
	} {
		decoded, err := decodeCallOptionsJSON([]byte(wire))
		require.NoError(t, err)
		assert.NotContains(t, decoded.ProviderOptions, "gateway")
	}

	for _, gateway := range []string{`null`, `[]`, `{"models":["fallback"]}`, `{"future":null}`} {
		_, err := decodeCallOptionsJSON([]byte(`{"prompt":[],"providerOptions":{"gateway":` + gateway + `}}`))
		require.Error(t, err)
	}

	empty := provider.CallOptions{ProviderOptions: provider.ProviderOptions{
		"gateway":  provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(`{}`)},
		"provider": provider.RawProviderOption{Key: "provider", Raw: json.RawMessage(`{"keep":true}`)},
	}}
	encoded, err := EncodeCallOptions(empty)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"gateway"`)

	nonEmpty := empty
	nonEmpty.ProviderOptions = provider.ProviderOptions{"gateway": provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(`{"models":["fallback"]}`)}}
	_, err = EncodeCallOptions(nonEmpty)
	require.Error(t, err)
}

func TestProviderReferenceValidationInRequestAndToolResultFiles(t *testing.T) {
	invalid := []json.RawMessage{
		json.RawMessage(`null`),
		json.RawMessage(`[]`),
		json.RawMessage(`{"p":null}`),
		json.RawMessage(`{"p":1}`),
		json.RawMessage(`{"type":"file-id"}`),
	}
	for i, reference := range invalid {
		t.Run(fmt.Sprintf("request-encode-%d", i), func(t *testing.T) {
			options := provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("application/pdf", provider.DataContent{Reference: reference}))}}
			_, err := EncodeCallOptions(options)
			require.Error(t, err)
		})
		t.Run(fmt.Sprintf("tool-result-encode-%d", i), func(t *testing.T) {
			output := &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentFile, Data: &provider.DataContent{Reference: reference}, MediaType: "application/pdf"}}}
			options := provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", output))}}
			_, err := EncodeCallOptions(options)
			require.Error(t, err)
		})
	}

	decodeCases := []string{
		`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"p":null}},"mediaType":"application/pdf"}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"file","data":{"type":"reference","reference":{"type":"id"}},"mediaType":"application/pdf"}]}}]}]}`,
	}
	for i, wire := range decodeCases {
		t.Run(fmt.Sprintf("decode-%d", i), func(t *testing.T) {
			_, err := decodeCallOptionsJSON([]byte(wire))
			require.Error(t, err)
		})
	}
}

func TestCallOptions_ToolResultOutputVariants(t *testing.T) {
	cases := []struct {
		output provider.ToolResultOutput
		wire   string
	}{
		{output: provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}, wire: `{"type":"text","value":"ok"}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "boom"}, wire: `{"type":"error-text","value":"boom"}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"ok":true}`)}, wire: `{"type":"json","value":{"ok":true}}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`null`)}, wire: `{"type":"json","value":null}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"error":true}`)}, wire: `{"type":"error-json","value":{"error":true}}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`null`)}, wire: `{"type":"error-json","value":null}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: "value"}}}, wire: `{"type":"content","value":[{"type":"text","text":"value"}]}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: "denied"}, wire: `{"type":"execution-denied","reason":"denied"}`},
	}
	for _, tc := range cases {
		t.Run(string(tc.output.Type), func(t *testing.T) {
			options := provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", &tc.output))}}
			data, err := EncodeCallOptions(options)
			require.NoError(t, err)
			var encoded struct {
				Prompt []struct {
					Content []struct {
						Output json.RawMessage `json:"output"`
					} `json:"content"`
				} `json:"prompt"`
			}
			require.NoError(t, json.Unmarshal(data, &encoded))
			assert.JSONEq(t, tc.wire, string(encoded.Prompt[0].Content[0].Output))

			wire := `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":` + tc.wire + `}]}]}`
			decoded, err := decodeCallOptionsJSON([]byte(wire))
			require.NoError(t, err)
			assert.Equal(t, options, decoded)
		})
	}
}

func TestCallOptions_ProviderToolRequiresCanonicalArgs(t *testing.T) {
	options := provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeProvider, Name: "tool", ID: "p.tool", Args: map[string]json.RawMessage{}}}}
	data, err := EncodeCallOptions(options)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[],"tools":[{"type":"provider","name":"tool","id":"p.tool","args":{}}]}`, string(data))
	decoded, err := decodeCallOptionsJSON(data)
	require.NoError(t, err)
	require.Len(t, decoded.Tools, 1)
	assert.NotNil(t, decoded.Tools[0].Args)
	assert.Empty(t, decoded.Tools[0].Args)

	invalid := []string{
		`{"type":"provider","name":"tool","id":"p.tool"}`,
		`{"type":"provider","name":"tool","id":"p.tool","args":null}`,
		`{"type":"provider","name":"tool","id":"p.tool","args":{},"providerOptions":null}`,
	}
	for i, tool := range invalid {
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
			_, err := decodeCallOptionsJSON([]byte(`{"prompt":[],"tools":[` + tool + `]}`))
			require.Error(t, err)
		})
	}
}

func TestCallOptions_FunctionToolExamplesRequireObjects(t *testing.T) {
	for _, input := range []string{`null`, `1`, `[]`, `"value"`} {
		t.Run(input, func(t *testing.T) {
			wire := `{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":{},"inputExamples":[{"input":` + input + `}]}]}`
			_, err := decodeCallOptionsJSON([]byte(wire))
			require.Error(t, err)
		})
	}
	valid := `{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":{},"inputExamples":[{"input":{"value":1}}]}]}`
	decoded, err := decodeCallOptionsJSON([]byte(valid))
	require.NoError(t, err)
	encoded, err := EncodeCallOptions(decoded)
	require.NoError(t, err)
	assert.JSONEq(t, valid, string(encoded))
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

			quoted, err := json.Marshal(value)
			require.NoError(t, err)
			unary := `{"content":[{"type":"tool-call","toolCallId":"call","toolName":"tool","input":` + string(quoted) + `}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`
			_, err = DecodeGenerateResult([]byte(unary))
			require.Error(t, err)
			_, err = encodeGenerateResultJSON(&provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "tool", Input: json.RawMessage(value)}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Warnings: []provider.Warning{}})
			require.Error(t, err)

			stream := `{"type":"tool-call","toolCallId":"call","toolName":"tool","input":` + string(quoted) + `}`
			_, err = DecodeStreamPart([]byte(stream))
			require.Error(t, err)
			_, err = encodeStreamPartJSON(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "tool", Input: value})
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
	result := &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "tool", Input: json.RawMessage(`{}`)}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Warnings: []provider.Warning{}}
	encoded, err = encodeGenerateResultJSON(result)
	require.NoError(t, err)
	_, err = DecodeGenerateResult(encoded)
	require.NoError(t, err)
	encoded, err = encodeStreamPartJSON(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "tool", Input: `{}`})
	require.NoError(t, err)
	_, err = DecodeStreamPart(encoded)
	require.NoError(t, err)
}

func TestCallOptions_StrictRejectionAndAdditiveFields(t *testing.T) {
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
		{name: "legacy tool file", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"file-data","data":"AAEC","mediaType":"x"}]}}]}]}`},
		{name: "unknown file data", wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"future"},"mediaType":"x"}]}]}`},
		{name: "invalid tool choice", wire: `{"prompt":[],"toolChoice":{"type":"tool"}}`},
		{name: "provider option scalar", wire: `{"prompt":[],"providerOptions":{"provider":1}}`},
		{name: "provider tool options", wire: `{"prompt":[],"tools":[{"type":"provider","id":"p.tool","name":"tool","args":{},"providerOptions":{"p":{}}}]}`},
		{name: "provider tool empty options", wire: `{"prompt":[],"tools":[{"type":"provider","id":"p.tool","name":"tool","args":{},"providerOptions":{}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeCallOptionsJSON([]byte(tc.wire))
			require.Error(t, err)
		})
	}

	additive := `{"prompt":[{"role":"user","content":[{"type":"text","text":"x","data":false}]}],"tools":[{"type":"function","name":"tool","inputSchema":{},"args":false}],"toolChoice":{"type":"none","toolName":false},"responseFormat":{"type":"text","schema":false},"futureField":{"safe":true}}`
	decoded, err := decodeCallOptionsJSON([]byte(additive))
	require.NoError(t, err)
	assert.Empty(t, decoded.Prompt[0].Content[0].Data)
	assert.Empty(t, decoded.Tools[0].Args)
	assert.Empty(t, decoded.ToolChoice.ToolName)
	assert.Empty(t, decoded.ResponseFormat.Schema)
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

func TestGenerateResult_AllCanonicalVariantsRoundTrip(t *testing.T) {
	preliminary := false
	dynamic := true
	timestamp := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "hello"},
			{Type: provider.ContentReasoning, Text: "thinking"},
			{Type: provider.ContentToolCall, ToolCallID: "call-1", ToolName: "search", Input: json.RawMessage(`{"q":"go"}`), ProviderExecuted: true, Dynamic: &dynamic},
			{Type: provider.ContentToolResult, ToolCallID: "call-1", ToolName: "search", Result: json.RawMessage(`{"ok":true}`), Preliminary: &preliminary, Dynamic: &dynamic},
			{Type: provider.ContentSource, ID: "source-1", SourceType: provider.SourceTypeURL, URL: "https://example.com", Title: "Example"},
			{Type: provider.ContentSource, ID: "source-2", SourceType: provider.SourceTypeDocument, Title: "Document", MediaType: "application/pdf", Filename: "document.pdf"},
			{Type: provider.ContentFile, MediaType: "image/png", Data: &provider.DataContent{Base64: "AAEC"}},
			{Type: provider.ContentReasoningFile, MediaType: "image/png", Data: &provider.DataContent{URL: "https://example.com/reasoning.png"}},
			{Type: provider.ContentCustom, Kind: "provider.custom"},
			{Type: provider.ContentToolApprovalRequest, ApprovalID: "approval-1", ToolCallID: "call-2"},
		},
		FinishReason:     provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"},
		Usage:            provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPointer(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPointer(5)}, Raw: json.RawMessage(`{"input":10}`)},
		ProviderMetadata: provider.ProviderMetadata{"provider": json.RawMessage(`{"keep":true}`)},
		Warnings:         []provider.Warning{{Type: provider.WarnUnsupported, Feature: "feature"}},
		Request:          &provider.RequestMetadata{Body: json.RawMessage(`{"private":true}`)},
		Response:         &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "response-1", ModelID: "backend-model", Timestamp: timestamp}, Headers: map[string]string{"x-id": "id"}, Body: json.RawMessage(`{"raw":true}`)},
	}

	data, err := encodeGenerateResultJSON(result)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"input":"{\"q\":\"go\"}"`)
	decoded, err := DecodeGenerateResult(data)
	require.NoError(t, err)
	assert.Equal(t, result, decoded)
}

func TestGenerateResult_PinnedCanonicalGolden(t *testing.T) {
	golden := `{
		"content":[
			{"type":"text","text":"hello"},
			{"type":"reasoning","text":"thinking"},
			{"type":"tool-call","toolCallId":"call","toolName":"search","input":"{\"q\":\"go\"}"},
			{"type":"tool-result","toolCallId":"call","toolName":"search","result":{"ok":true}},
			{"type":"source","sourceType":"url","id":"source","url":"https://example.com"},
			{"type":"source","sourceType":"document","id":"document","title":"Document","mediaType":"application/pdf","filename":"document.pdf"},
			{"type":"file","data":{"type":"data","data":"AAEC"},"mediaType":"image/png"},
			{"type":"reasoning-file","data":{"type":"url","url":"https://example.com/reasoning.png"},"mediaType":"image/png"},
			{"type":"custom","kind":"provider.custom"},
			{"type":"tool-approval-request","approvalId":"approval","toolCallId":"call-2"}
		],
		"finishReason":{"unified":"stop","raw":"end_turn"},
		"usage":{"inputTokens":{"total":1},"outputTokens":{"total":2}},
		"providerMetadata":{"provider":{"keep":true}},
		"warnings":[],
		"request":{"body":{"request":true}},
		"response":{"id":"response","modelId":"backend","timestamp":"2026-04-01T10:00:00Z","headers":{"x-id":"id"},"body":{"response":true}}
	}`
	decoded, err := DecodeGenerateResult([]byte(golden))
	require.NoError(t, err)
	encoded, err := encodeGenerateResultJSON(decoded)
	require.NoError(t, err)
	assert.JSONEq(t, golden, string(encoded))
}

func TestUsage_RequiresTokenObjects(t *testing.T) {
	validUsage := `{"inputTokens":{},"outputTokens":{}}`
	unary := `{"content":[],"finishReason":{"unified":"stop"},"usage":` + validUsage + `,"warnings":[]}`
	decoded, err := DecodeGenerateResult([]byte(unary))
	require.NoError(t, err)
	assert.Equal(t, provider.Usage{}, decoded.Usage)
	encoded, err := encodeGenerateResultJSON(decoded)
	require.NoError(t, err)
	assert.JSONEq(t, unary, string(encoded))

	stream := `{"type":"finish","usage":` + validUsage + `,"finishReason":{"unified":"stop"}}`
	part, err := DecodeStreamPart([]byte(stream))
	require.NoError(t, err)
	require.NotNil(t, part.Usage)
	assert.Equal(t, provider.Usage{}, *part.Usage)
	encoded, err = encodeStreamPartJSON(part)
	require.NoError(t, err)
	assert.JSONEq(t, stream, string(encoded))

	invalid := []string{
		`{}`,
		`{"inputTokens":{}}`,
		`{"outputTokens":{}}`,
		`{"inputTokens":null,"outputTokens":{}}`,
		`{"inputTokens":{},"outputTokens":null}`,
		`{"inputTokens":1,"outputTokens":{}}`,
		`{"inputTokens":{},"outputTokens":1}`,
		`{"inputTokens":[],"outputTokens":{}}`,
		`{"inputTokens":{},"outputTokens":[]}`,
	}
	for i, usage := range invalid {
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
			wire := `{"content":[],"finishReason":{"unified":"stop"},"usage":` + usage + `,"warnings":[]}`
			_, err := DecodeGenerateResult([]byte(wire))
			require.Error(t, err)

			wire = `{"type":"finish","usage":` + usage + `,"finishReason":{"unified":"stop"}}`
			_, err = DecodeStreamPart([]byte(wire))
			require.Error(t, err)
		})
	}
}

func TestUsage_RawRequiresJSONObject(t *testing.T) {
	valid := &provider.GenerateResult{
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage:        provider.Usage{Raw: json.RawMessage(`{"providerTokens":1}`)},
		Warnings:     []provider.Warning{},
	}
	data, err := encodeGenerateResultJSON(valid)
	require.NoError(t, err)
	decoded, err := DecodeGenerateResult(data)
	require.NoError(t, err)
	assert.JSONEq(t, `{"providerTokens":1}`, string(decoded.Usage.Raw))

	for _, raw := range []string{`null`, `1`, `[]`, `"value"`} {
		t.Run(raw, func(t *testing.T) {
			invalid := *valid
			invalid.Usage.Raw = json.RawMessage(raw)
			_, err := encodeGenerateResultJSON(&invalid)
			require.Error(t, err)

			wire := `{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{},"raw":` + raw + `},"warnings":[]}`
			_, err = DecodeGenerateResult([]byte(wire))
			require.Error(t, err)
		})
	}
}

func TestWarnings_AllPinnedVariantsAndRequiredEmptyStrings(t *testing.T) {
	warnings := []provider.Warning{
		{Type: provider.WarnUnsupported, Feature: "", Details: "detail"},
		{Type: provider.WarnCompatibility, Feature: "compat"},
		{Type: provider.WarnDeprecated, Setting: "", Message: ""},
		{Type: provider.WarnOther, Message: ""},
	}
	result := &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Warnings: warnings}
	data, err := encodeGenerateResultJSON(result)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"feature":""`)
	assert.Contains(t, string(data), `"setting":"","message":""`)
	decoded, err := DecodeGenerateResult(data)
	require.NoError(t, err)
	assert.Equal(t, warnings, decoded.Warnings)

	invalid := []string{
		`{"type":"unsupported"}`,
		`{"type":"compatibility"}`,
		`{"type":"deprecated","setting":"x"}`,
		`{"type":"deprecated","message":"x"}`,
		`{"type":"other"}`,
	}
	for _, warning := range invalid {
		wire := `{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[` + warning + `]}`
		_, err := DecodeGenerateResult([]byte(wire))
		require.Error(t, err)
	}
}

func TestGenerateResult_StrictRejections(t *testing.T) {
	valid := `{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`
	cases := []string{
		`{}`,
		`{"content":[{"type":"future"}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`,
		`{"content":[{"type":"tool-call","toolCallId":"c","toolName":"t","input":"{"}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`,
		`{"content":[],"finishReason":{"unified":"future"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`,
		`{"content":[{"type":"file","data":{"type":"data","data":"AAEC"},"mediaType":"image/png","filename":"x.png"}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"providerMetadata":{"p":1}}`,
	}
	for _, wire := range cases {
		_, err := DecodeGenerateResult([]byte(wire))
		require.Error(t, err)
	}
	decoded, err := DecodeGenerateResult([]byte(valid[:len(valid)-1] + `,"future":true}`))
	require.NoError(t, err)
	assert.Empty(t, decoded.Content)

	_, err = encodeGenerateResultJSON(&provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "c", ToolName: "t", Input: json.RawMessage(`{`)}}})
	require.Error(t, err)
	_, err = encodeGenerateResultJSON(&provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentFile, Filename: "x.png", MediaType: "image/png", Data: &provider.DataContent{Base64: "AAEC"}}}})
	require.Error(t, err)
}

func TestStreamPart_AllCanonicalDiscriminatorsRoundTrip(t *testing.T) {
	usage := provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPointer(1)}, OutputTokens: provider.OutputTokenUsage{Total: intPointer(2)}}
	finish := provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"}
	preliminary := false
	dynamic := true
	timestamp := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "text"},
		{Type: provider.PartTextDelta, ID: "text", Delta: ""},
		{Type: provider.PartTextEnd, ID: "text"},
		{Type: provider.PartReasoningStart, ID: "reasoning"},
		{Type: provider.PartReasoningDelta, ID: "reasoning", Delta: "thinking"},
		{Type: provider.PartReasoningEnd, ID: "reasoning"},
		{Type: provider.PartToolInputStart, ID: "input", ToolName: "search", ProviderExecuted: true, Dynamic: &dynamic, Title: "Search"},
		{Type: provider.PartToolInputDelta, ID: "input", Delta: "{}"},
		{Type: provider.PartToolInputEnd, ID: "input"},
		{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "search", Input: `{"q":"go"}`, ProviderExecuted: true, Dynamic: &dynamic},
		{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "search", Result: json.RawMessage(`{"ok":true}`), Preliminary: &preliminary, Dynamic: &dynamic},
		{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "source", URL: "https://example.com", Title: "Example", ProviderMetadata: provider.ProviderMetadata{"p": json.RawMessage(`{"x":1}`)}}},
		{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "document", Title: "Document", MediaType: "application/pdf", Filename: "document.pdf"}},
		{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Base64: "AAEC"}, MediaType: "image/png"},
		{Type: provider.PartStreamStart, Warnings: []provider.Warning{}},
		{Type: provider.PartResponseMeta, ResponseID: "response", ModelID: "model", Timestamp: timestamp},
		{Type: provider.PartFinish, Usage: &usage, FinishReason: &finish},
		{Type: provider.PartRaw, RawValue: json.RawMessage(`{"raw":true}`)},
		{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "safe", StatusCode: 429, IsRetryable: boolPointer(true), Data: json.RawMessage(`{"type":"rate_limit_exceeded"}`)})},
		{Type: provider.PartToolApprovalRequest, ApprovalID: "approval", ToolCallID: "call"},
		{Type: provider.PartCustom, Kind: "provider.custom"},
		{Type: provider.PartReasoningFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "https://example.com/reasoning.png"}, MediaType: "image/png"},
	}

	for _, part := range parts {
		t.Run(string(part.Type), func(t *testing.T) {
			data, err := encodeStreamPartJSON(part)
			require.NoError(t, err)
			if part.Type == provider.PartTextDelta {
				assert.Contains(t, string(data), `"delta":""`)
			}
			decoded, err := DecodeStreamPart(data)
			require.NoError(t, err)
			assert.Equal(t, part, decoded)
		})
	}
}

func TestStreamPart_PinnedCanonicalGoldens(t *testing.T) {
	goldens := []string{
		`{"type":"text-start","id":"text"}`,
		`{"type":"text-delta","id":"text","delta":""}`,
		`{"type":"text-end","id":"text"}`,
		`{"type":"reasoning-start","id":"reasoning"}`,
		`{"type":"reasoning-delta","id":"reasoning","delta":"thinking"}`,
		`{"type":"reasoning-end","id":"reasoning"}`,
		`{"type":"tool-input-start","id":"input","toolName":"search","providerExecuted":true,"dynamic":true,"title":"Search"}`,
		`{"type":"tool-input-delta","id":"input","delta":"{}"}`,
		`{"type":"tool-input-end","id":"input"}`,
		`{"type":"tool-call","toolCallId":"call","toolName":"search","input":"{\"q\":\"go\"}","providerExecuted":true,"dynamic":true}`,
		`{"type":"tool-result","toolCallId":"call","toolName":"search","result":{"ok":true},"preliminary":false,"dynamic":true}`,
		`{"type":"source","sourceType":"url","id":"source","url":"https://example.com","title":"Example"}`,
		`{"type":"source","sourceType":"document","id":"document","title":"Document","mediaType":"application/pdf","filename":"document.pdf"}`,
		`{"type":"file","data":{"type":"data","data":"AAEC"},"mediaType":"image/png"}`,
		`{"type":"stream-start","warnings":[]}`,
		`{"type":"response-metadata","id":"response","modelId":"model","timestamp":"2026-04-01T10:00:00Z"}`,
		`{"type":"finish","usage":{"inputTokens":{"total":1},"outputTokens":{"total":2}},"finishReason":{"unified":"stop","raw":"end_turn"}}`,
		`{"type":"raw","rawValue":{"raw":true}}`,
		`{"type":"error","error":{"message":"safe","statusCode":429,"isRetryable":true,"data":{"type":"rate_limit_exceeded"}}}`,
		`{"type":"tool-approval-request","approvalId":"approval","toolCallId":"call"}`,
		`{"type":"custom","kind":"provider.custom"}`,
		`{"type":"reasoning-file","data":{"type":"url","url":"https://example.com/reasoning.png"},"mediaType":"image/png"}`,
	}
	for _, golden := range goldens {
		var discriminator struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal([]byte(golden), &discriminator))
		t.Run(discriminator.Type, func(t *testing.T) {
			part, err := DecodeStreamPart([]byte(golden))
			require.NoError(t, err)
			encoded, err := encodeStreamPartJSON(part)
			require.NoError(t, err)
			assert.JSONEq(t, golden, string(encoded))
		})
	}
}

func TestStreamPart_StrictRejectionsAndAdditiveFields(t *testing.T) {
	cases := []string{
		`{"type":"future"}`,
		`{"type":"text-delta","id":"text"}`,
		`{"type":"tool-call","toolCallId":"c","toolName":"t","input":"{"}`,
		`{"type":"source","source":{"sourceType":"url","id":"s","url":"https://example.com"}}`,
		`{"type":"file","fileData":"AAEC","mediaType":"image/png"}`,
		`{"type":"error","apiCallError":{"message":"legacy"}}`,
		`{"type":"finish"}`,
		`{"type":"file","data":{"type":"data","data":"AAEC"},"mediaType":"image/png","filename":"x.png"}`,
		`{"type":"stream-start","warnings":[{"type":"other"}]}`,
	}
	for _, wire := range cases {
		_, err := DecodeStreamPart([]byte(wire))
		require.Error(t, err)
	}
	part, err := DecodeStreamPart([]byte(`{"type":"text-delta","id":"text","delta":"x","future":true}`))
	require.NoError(t, err)
	assert.Equal(t, "x", part.Delta)
}

func TestFailureProjectionAndSanitization(t *testing.T) {
	private := provider.NewAPICallError(provider.APICallErrorOptions{
		Message: "private backend detail", URL: "https://backend", StatusCode: 503,
		RequestBodyValues: json.RawMessage(`{"secret":true}`), ResponseHeaders: map[string][]string{"X-Secret": {"secret"}},
		ResponseBody: "private body", Data: json.RawMessage(`{"private":true}`),
	})
	failure := classifyProviderError(private)
	status, body, err := encodeFailure(failure)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadGateway, status)
	assert.NotContains(t, string(body), "private")
	assert.NotContains(t, string(body), "backend")
	apiErr, err := DecodeErrorResponse(body, status)
	require.NoError(t, err)
	assert.Equal(t, "upstream dependency failed", apiErr.Message)
	assert.True(t, apiErr.IsRetryable)

	safe := sanitizePartError(private)
	assert.Equal(t, http.StatusBadGateway, safe.StatusCode)
	assert.NotContains(t, safe.Message, "private")
	assert.NotContains(t, string(safe.Data), "private")
	assert.ErrorIs(t, safe, private)
}

func TestDecodeErrorResponse_StrictValidation(t *testing.T) {
	cases := []string{
		`{"error":{"message":"x","type":"internal_server_error","statusCode":500}}`,
		`{"error":{"message":"x","type":"future_error","statusCode":500,"isRetryable":false}}`,
		`{"error":{"message":"x","type":"internal_server_error","statusCode":500,"isRetryable":null}}`,
		`{"error":{"message":"x","type":"internal_server_error","statusCode":200,"isRetryable":false}}`,
		`{"error":{"message":"x","type":"internal_server_error","statusCode":600,"isRetryable":false}}`,
	}
	for _, wire := range cases {
		_, err := DecodeErrorResponse([]byte(wire), 500)
		require.Error(t, err)
	}
}

func TestEncodedLimitsAndBoundedSSEReader(t *testing.T) {
	part := provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "x"}
	event, err := encodeSSEEventWithinLimit(part, 1024)
	require.NoError(t, err)
	atLimit, err := encodeSSEEventWithinLimit(part, int64(len(event)))
	require.NoError(t, err)
	assert.Equal(t, event, atLimit)
	_, err = encodeSSEEventWithinLimit(part, int64(len(event)-1))
	assert.ErrorIs(t, err, errSSEEventTooLarge)

	reader, err := NewSSEReader(bytes.NewReader(event), int64(len(event)))
	require.NoError(t, err)
	decoded, err := reader.Next()
	require.NoError(t, err)
	assert.Equal(t, part, decoded)
	_, err = reader.Next()
	assert.ErrorIs(t, err, io.EOF)

	reader, err = NewSSEReader(bytes.NewReader(event), int64(len(event)-1))
	require.NoError(t, err)
	_, err = reader.Next()
	assert.ErrorIs(t, err, errSSEEventTooLarge)

	multiline := "data: {\"type\":\"text-delta\",\n" + "data: \"id\":\"text\",\"delta\":\"x\"}\n\n"
	reader, err = NewSSEReader(bytes.NewBufferString(multiline), int64(len(multiline)))
	require.NoError(t, err)
	decoded, err = reader.Next()
	require.NoError(t, err)
	assert.Equal(t, part, decoded)

	unterminated := bytes.TrimSuffix(event, []byte("\n\n"))
	reader, err = NewSSEReader(bytes.NewReader(unterminated), int64(len(unterminated)))
	require.NoError(t, err)
	decoded, err = reader.Next()
	require.NoError(t, err)
	assert.Equal(t, part, decoded)
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
	}
	encoded, err := EncodeCallOptions(options)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[{"role":"user","content":[{"type":"text","text":"text"}]}],"tools":[{"type":"function","name":"tool","inputSchema":{}}]}`, string(encoded))

	_, err = EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(
		provider.FilePart("text/plain", provider.DataContent{Bytes: []byte{}, URL: "https://example.com"}),
	)}})
	require.Error(t, err)
}

func TestCallOptions_NestedGatewayProviderOptionsAreRejected(t *testing.T) {
	gatewayOptions := provider.ProviderOptions{"gateway": provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(`{"models":["fallback"]}`)}}
	validOutput := provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}
	encodeCases := []struct {
		name    string
		options provider.CallOptions
	}{
		{name: "message", options: provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleUser, Content: []provider.ContentPart{}, ProviderOptions: gatewayOptions}}}},
		{name: "content part", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeText, Text: "text", ProviderOptions: gatewayOptions})}}},
		{name: "function tool", options: provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool", InputSchema: json.RawMessage(`{}`), ProviderOptions: gatewayOptions}}}},
		{name: "tool output", options: provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ContentPart{Type: provider.ContentPartTypeToolResult, ToolCallID: "call", ToolName: "tool", Output: &provider.ToolResultOutput{Type: validOutput.Type, Text: validOutput.Text, ProviderOptions: gatewayOptions}})}}},
		{name: "tool result content", options: provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: "text", ProviderOptions: gatewayOptions}}}))}}},
	}
	for _, tc := range encodeCases {
		t.Run("encode-"+tc.name, func(t *testing.T) {
			_, err := EncodeCallOptions(tc.options)
			require.Error(t, err)
		})
	}

	decodeCases := []struct {
		name string
		wire string
	}{
		{name: "message", wire: `{"prompt":[{"role":"user","content":[],"providerOptions":{"gateway":{}}}]}`},
		{name: "content part", wire: `{"prompt":[{"role":"user","content":[{"type":"text","text":"text","providerOptions":{"gateway":{}}}]}]}`},
		{name: "function tool", wire: `{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":{},"providerOptions":{"gateway":{}}}]}`},
		{name: "tool output", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"text","value":"ok","providerOptions":{"gateway":{}}}}]}]}`},
		{name: "tool result content", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"text","providerOptions":{"gateway":{}}]}}]}]}`},
	}
	for _, tc := range decodeCases {
		t.Run("decode-"+tc.name, func(t *testing.T) {
			_, err := decodeCallOptionsJSON([]byte(tc.wire))
			require.Error(t, err)
		})
	}

	topLevel := provider.CallOptions{ProviderOptions: provider.ProviderOptions{
		"gateway": provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(`{"models":["fallback"]}`)},
	}}
	_, err := EncodeCallOptions(topLevel)
	require.Error(t, err)

	nestedUnknown := `{"prompt":[{"role":"user","content":[{"type":"text","text":"text","providerOptions":{"provider":{"part":true}}}],"providerOptions":{"provider":{"message":true}}},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"ok","providerOptions":{"provider":{"content":true}}}],"providerOptions":{"provider":{"output":true}}}}]}],"tools":[{"type":"function","name":"tool","inputSchema":{},"providerOptions":{"provider":{"tool":true}}}]}`
	decoded, err := decodeCallOptionsJSON([]byte(nestedUnknown))
	require.NoError(t, err)
	encoded, err := EncodeCallOptions(decoded)
	require.NoError(t, err)
	assert.JSONEq(t, nestedUnknown, string(encoded))
}

func TestStreamPart_ProviderMetadataEligibility(t *testing.T) {
	metadata := provider.ProviderMetadata{"p": json.RawMessage(`{}`)}
	ineligible := []provider.StreamPart{
		{Type: provider.PartStreamStart, Warnings: []provider.Warning{}, ProviderMetadata: metadata},
		{Type: provider.PartResponseMeta, ProviderMetadata: metadata},
		{Type: provider.PartRaw, RawValue: json.RawMessage(`null`), ProviderMetadata: metadata},
		{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "error", StatusCode: 500}), ProviderMetadata: metadata},
	}
	for _, part := range ineligible {
		t.Run("encode-ignored-"+string(part.Type), func(t *testing.T) {
			encoded, err := encodeStreamPartJSON(part)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "providerMetadata")
		})
	}

	for _, variant := range []string{"stream-start", "response-metadata", "raw", "error"} {
		base := map[string]string{
			"stream-start":      `{"type":"stream-start","warnings":[]}`,
			"response-metadata": `{"type":"response-metadata"}`,
			"raw":               `{"type":"raw","rawValue":null}`,
			"error":             `{"type":"error","error":{"message":"error","statusCode":500,"isRetryable":false}}`,
		}[variant]
		wire := strings.TrimSuffix(base, "}") + `,"providerMetadata":{"p":{}}}`
		t.Run("decode-ignored-"+variant, func(t *testing.T) {
			part, err := DecodeStreamPart([]byte(wire))
			require.NoError(t, err)
			assert.Empty(t, part.ProviderMetadata)
		})
	}

	allowed := []string{
		`{"type":"text-start","id":"id"}`, `{"type":"text-delta","id":"id","delta":""}`, `{"type":"text-end","id":"id"}`,
		`{"type":"reasoning-start","id":"id"}`, `{"type":"reasoning-delta","id":"id","delta":""}`, `{"type":"reasoning-end","id":"id"}`,
		`{"type":"tool-input-start","id":"id","toolName":"tool"}`, `{"type":"tool-input-delta","id":"id","delta":""}`, `{"type":"tool-input-end","id":"id"}`,
		`{"type":"tool-call","toolCallId":"call","toolName":"tool","input":"{}"}`,
		`{"type":"tool-result","toolCallId":"call","toolName":"tool","result":{}}`,
		`{"type":"source","sourceType":"url","id":"source","url":"https://example.com"}`,
		`{"type":"file","data":{"type":"data","data":"AA=="},"mediaType":"image/png"}`,
		`{"type":"reasoning-file","data":{"type":"data","data":"AA=="},"mediaType":"image/png"}`,
		`{"type":"finish","usage":{"inputTokens":{},"outputTokens":{}},"finishReason":{"unified":"stop"}}`,
		`{"type":"tool-approval-request","approvalId":"approval","toolCallId":"call"}`, `{"type":"custom","kind":"p.kind"}`,
	}
	for _, base := range allowed {
		wire := strings.TrimSuffix(base, "}") + `,"providerMetadata":{"p":{}}}`
		var discriminator struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal([]byte(base), &discriminator))
		t.Run("allowed-"+discriminator.Type, func(t *testing.T) {
			part, err := DecodeStreamPart([]byte(wire))
			require.NoError(t, err)
			if part.Type == provider.PartSource {
				assert.Equal(t, metadata, part.Source.ProviderMetadata)
			} else {
				assert.Equal(t, metadata, part.ProviderMetadata)
			}
		})
	}
}

func TestStrictCodec_RejectsTypedNullAndPrivateFields(t *testing.T) {
	requests := []string{
		`{"prompt":null}`, `{"prompt":[],"abortSignal":null}`, `{"prompt":[],"stopSequences":[null]}`, `{"prompt":[],"headers":{"x":null}}`,
		`{"prompt":[{"role":"user","content":[{"type":"text","text":null}]}]}`,
		`{"prompt":[{"role":"user","content":[{"type":"text","text":"x","sourceType":null}]}]}`,
		`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"p":null}},"mediaType":"x"}]}]}`,
		`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"type":"id"}},"mediaType":"x"}]}]}`,
	}
	for i, wire := range requests {
		t.Run(fmt.Sprintf("request-%d", i), func(t *testing.T) {
			_, err := decodeCallOptionsJSON([]byte(wire))
			require.Error(t, err)
		})
	}

	unarySuffix := `,"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`
	unary := []string{
		`{"content":[],"finishReason":{"unified":"stop","raw":null},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"provider":null}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"headers":{"x":null}}}`,
		`{"content":[{"type":"custom","kind":null}]` + unarySuffix,
	}
	for i, wire := range unary {
		t.Run(fmt.Sprintf("unary-%d", i), func(t *testing.T) {
			_, err := DecodeGenerateResult([]byte(wire))
			require.Error(t, err)
		})
	}

	stream := []string{
		`{"type":"text-delta","id":"id","delta":null}`,
		`{"type":"response-metadata","provider":null}`,
		`{"type":"response-metadata","responseHeaders":null}`,
		`{"type":"tool-approval-request","approvalId":"approval","toolCallId":"call","signature":null}`,
		`{"type":"finish","usage":{"inputTokens":{},"outputTokens":{}},"finishReason":{"unified":"stop","raw":null}}`,
		`{"type":"error","error":{"message":"error","statusCode":500,"isRetryable":false,"url":null}}`,
	}
	for i, wire := range stream {
		t.Run(fmt.Sprintf("stream-%d", i), func(t *testing.T) {
			_, err := DecodeStreamPart([]byte(wire))
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

	part, err := DecodeStreamPart([]byte(`{"type":"raw","rawValue":null}`))
	require.NoError(t, err)
	assert.Equal(t, "null", string(part.RawValue))
}

func TestProviderQualifiedIdentifiers_EncodeAndDecode(t *testing.T) {
	invalid := []string{"", "tool", ".tool", "p.", " .tool", "p. "}
	for _, value := range invalid {
		t.Run(fmt.Sprintf("provider-tool-%q", value), func(t *testing.T) {
			_, err := EncodeCallOptions(provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: value, Name: "tool", Args: map[string]json.RawMessage{}}}})
			require.Error(t, err)
			_, err = decodeCallOptionsJSON([]byte(`{"prompt":[],"tools":[{"type":"provider","id":` + mustJSON(t, value) + `,"name":"tool","args":{}}]}`))
			require.Error(t, err)
		})
		for _, tc := range []struct {
			name   string
			encode func() error
			decode func() error
		}{
			{name: "request-custom", encode: func() error {
				_, err := encodeContentPart(provider.ContentPart{Type: provider.ContentPartTypeCustom, Kind: value})
				return err
			}, decode: func() error {
				_, err := decodeContentPart(json.RawMessage(`{"type":"custom","kind":` + mustJSON(t, value) + `}`))
				return err
			}},
			{name: "result-custom", encode: func() error {
				_, err := encodeGenerateContent(provider.GenerateContentPart{Type: provider.ContentCustom, Kind: value})
				return err
			}, decode: func() error {
				_, err := decodeGenerateContent(json.RawMessage(`{"type":"custom","kind":` + mustJSON(t, value) + `}`))
				return err
			}},
			{name: "stream-custom", encode: func() error {
				_, err := encodeStreamPartJSON(provider.StreamPart{Type: provider.PartCustom, Kind: value})
				return err
			}, decode: func() error {
				_, err := DecodeStreamPart([]byte(`{"type":"custom","kind":` + mustJSON(t, value) + `}`))
				return err
			}},
		} {
			t.Run(tc.name+fmt.Sprintf("-%q", value), func(t *testing.T) {
				require.Error(t, tc.encode())
				require.Error(t, tc.decode())
			})
		}
	}
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

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

func intPointer(value int) *int    { return &value }
func boolPointer(value bool) *bool { return &value }
