package providerwirev4

import (
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

func TestCallOptions_ToolResultOutputProviderOptionsEligibility(t *testing.T) {
	providerOptions := provider.ProviderOptions{
		"provider": provider.RawProviderOption{Key: "provider", Raw: json.RawMessage(`{"keep":true}`)},
	}
	eligible := []provider.ToolResultOutput{
		{Type: provider.ToolOutputText, Text: "ok", ProviderOptions: providerOptions},
		{Type: provider.ToolOutputErrorText, Text: "error", ProviderOptions: providerOptions},
		{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`null`), ProviderOptions: providerOptions},
		{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"error":true}`), ProviderOptions: providerOptions},
		{Type: provider.ToolOutputExecutionDenied, Reason: "denied", ProviderOptions: providerOptions},
	}
	for _, output := range eligible {
		t.Run(string(output.Type), func(t *testing.T) {
			options := provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", &output))}}
			encoded, err := EncodeCallOptions(options)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), `"providerOptions":{"provider":{"keep":true}}`)
			decoded, err := decodeCallOptionsJSON(encoded)
			require.NoError(t, err)
			assert.Equal(t, options, decoded)
		})
	}

	content := provider.ToolResultOutput{
		Type:            provider.ToolOutputContent,
		Content:         []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: "ok"}},
		ProviderOptions: providerOptions,
	}
	_, err := EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{
		provider.NewToolMessage(provider.ToolResultPart("call", "tool", &content)),
	}})
	require.Error(t, err)

	wire := `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"ok","providerOptions":{"provider":{"nested":true}}}],"providerOptions":{"provider":{"inactive":true}}}}]}]}`
	decoded, err := decodeCallOptionsJSON([]byte(wire))
	require.NoError(t, err)
	output := decoded.Prompt[0].Content[0].Output
	require.NotNil(t, output)
	assert.Empty(t, output.ProviderOptions)
	assert.Contains(t, output.Content[0].ProviderOptions, "provider")
	encoded, err := EncodeCallOptions(decoded)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"ok","providerOptions":{"provider":{"nested":true}}}]}}]}]}`, string(encoded))
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
	_, err = EncodeCallOptions(provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeProvider, Name: "tool", ID: "p.tool"}}})
	require.Error(t, err)
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

	strict := false
	explicitFalse := provider.CallOptions{Prompt: []provider.Message{}, Tools: []provider.Tool{{
		Type: provider.ToolTypeFunction, Name: "tool", InputSchema: json.RawMessage(`{}`), InputExamples: []provider.InputExample{}, Strict: &strict,
	}}}
	encoded, err = EncodeCallOptions(explicitFalse)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":{},"strict":false}]}`, string(encoded))
	decoded, err = decodeCallOptionsJSON(encoded)
	require.NoError(t, err)
	assert.Equal(t, explicitFalse, decoded)
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
		{name: "legacy tool file data", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"file-data","data":"AAEC","mediaType":"x"}]}}]}]}`},
		{name: "legacy tool file URL", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"file-url","url":"https://example.com","mediaType":"x"}]}}]}]}`},
		{name: "legacy tool file reference", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"file-reference","reference":{"p":"id"},"mediaType":"x"}]}}]}]}`},
		{name: "unknown file data", wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"future"},"mediaType":"x"}]}]}`},
		{name: "unknown tool", wire: `{"prompt":[],"tools":[{"type":"future","name":"tool"}]}`},
		{name: "unknown tool output", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"future"}}]}]}`},
		{name: "unknown tool result content", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"future"}]}}]}]}`},
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
		{name: "inactive content tool output gateway", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[],"providerOptions":{"gateway":{}}}}]}]}`},
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

	nestedUnknown := `{"prompt":[{"role":"user","content":[{"type":"text","text":"text","providerOptions":{"provider":{"part":true}}}],"providerOptions":{"provider":{"message":true}}},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"ok","providerOptions":{"provider":{"content":true}}}],"providerOptions":{"provider":{"inactive":true}}}}]}],"tools":[{"type":"function","name":"tool","inputSchema":{},"providerOptions":{"provider":{"tool":true}}}]}`
	decoded, err := decodeCallOptionsJSON([]byte(nestedUnknown))
	require.NoError(t, err)
	encoded, err := EncodeCallOptions(decoded)
	require.NoError(t, err)
	expected := `{"prompt":[{"role":"user","content":[{"type":"text","text":"text","providerOptions":{"provider":{"part":true}}}],"providerOptions":{"provider":{"message":true}}},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"ok","providerOptions":{"provider":{"content":true}}}]}}]}],"tools":[{"type":"function","name":"tool","inputSchema":{},"providerOptions":{"provider":{"tool":true}}}]}`
	assert.JSONEq(t, expected, string(encoded))
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

}

func TestStrictCodec_AllowsNullOnlyForOpaqueNullableValues(t *testing.T) {
	request := `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"tool","input":null}]},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"json","value":null}}]}]}`
	decoded, err := decodeCallOptionsJSON([]byte(request))
	require.NoError(t, err)
	assert.Equal(t, "null", string(decoded.Prompt[0].Content[0].Input))
	assert.Equal(t, "null", string(decoded.Prompt[1].Content[0].Output.JSON))
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
		t.Run(fmt.Sprintf("request-custom-%q", value), func(t *testing.T) {
			_, err := encodeContentPart(provider.ContentPart{Type: provider.ContentPartTypeCustom, Kind: value})
			require.Error(t, err)
			_, err = decodeContentPart(json.RawMessage(`{"type":"custom","kind":` + mustJSON(t, value) + `}`))
			require.Error(t, err)
		})
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

func TestCallOptions_RoleContentMatrix(t *testing.T) {
	approved := false
	textOutput := &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}
	contentOutput := &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{
		{Type: provider.ToolContentText, Text: "text"},
		{Type: provider.ToolContentFile, Data: &provider.DataContent{URL: "https://example.com/file"}, MediaType: "application/octet-stream"},
		{Type: provider.ToolContentCustom},
	}}
	cases := []struct {
		name    string
		message provider.Message
	}{
		{name: "user text", message: provider.NewUserMessage(provider.TextPart("text"))},
		{name: "user file", message: provider.NewUserMessage(provider.FilePart("text/plain", provider.DataContent{Text: "file"}))},
		{name: "assistant text", message: provider.NewAssistantMessage(provider.TextPart("text"))},
		{name: "assistant file", message: provider.NewAssistantMessage(provider.FilePart("text/plain", provider.DataContent{Text: "file"}))},
		{name: "assistant custom", message: provider.NewAssistantMessage(provider.CustomPart("provider.custom"))},
		{name: "assistant reasoning", message: provider.NewAssistantMessage(provider.ReasoningPart("reasoning"))},
		{name: "assistant reasoning file", message: provider.NewAssistantMessage(provider.ReasoningFilePart("image/png", provider.DataContent{Base64: "AQ=="}))},
		{name: "assistant tool call", message: provider.NewAssistantMessage(provider.ToolCallPart("call", "tool", json.RawMessage(`null`)))},
		{name: "assistant tool result", message: provider.NewAssistantMessage(provider.ToolResultPart("call", "tool", textOutput))},
		{name: "tool result", message: provider.NewToolMessage(provider.ToolResultPart("call", "tool", contentOutput))},
		{name: "tool approval response", message: provider.NewToolMessage(provider.ContentPart{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval", Approved: &approved})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options := provider.CallOptions{Prompt: []provider.Message{tc.message}}
			encoded, err := EncodeCallOptions(options)
			require.NoError(t, err)
			decoded, err := decodeCallOptionsJSON(encoded)
			require.NoError(t, err)
			assert.Equal(t, options, decoded)
		})
	}

	system := provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleSystem, Content: []provider.ContentPart{provider.TextPart("a"), provider.TextPart("b")}}}}
	encoded, err := EncodeCallOptions(system)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[{"role":"system","content":"ab"}]}`, string(encoded))
}

func TestCallOptions_EmptyInlineDataAndReasoningFileRestrictions(t *testing.T) {
	emptyData := provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(
		provider.FilePart("application/octet-stream", provider.DataContent{Bytes: []byte{}}),
	)}}
	encoded, err := EncodeCallOptions(emptyData)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"data","data":""},"mediaType":"application/octet-stream"}]}]}`, string(encoded))
	decoded, err := decodeCallOptionsJSON(encoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.Prompt[0].Content[0].Data.Bytes)
	assert.Empty(t, decoded.Prompt[0].Content[0].Data.Bytes)

	for _, data := range []provider.DataContent{
		{Reference: json.RawMessage(`{"provider":"file"}`)},
		{Text: "inline"},
	} {
		_, err := EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{
			provider.NewAssistantMessage(provider.ReasoningFilePart("text/plain", data)),
		}})
		require.Error(t, err)
	}
	for _, variant := range []string{
		`{"type":"reference","reference":{"provider":"file"}}`,
		`{"type":"text","text":"inline"}`,
	} {
		wire := `{"prompt":[{"role":"assistant","content":[{"type":"reasoning-file","data":` + variant + `,"mediaType":"text/plain"}]}]}`
		_, err := decodeCallOptionsJSON([]byte(wire))
		require.Error(t, err)
	}
}

func TestCallOptions_PrivateFieldsAndRequiredIdentifiers(t *testing.T) {
	for _, field := range []string{"sourceType", "id", "url", "title", "signature", "isAutomatic"} {
		t.Run("decode-content-"+field, func(t *testing.T) {
			wire := `{"prompt":[{"role":"user","content":[{"type":"text","text":"x","` + field + `":null}]}]}`
			_, err := decodeCallOptionsJSON([]byte(wire))
			require.Error(t, err)
		})
	}
	for _, field := range []string{"toolCallId", "toolName", "providerExecuted"} {
		t.Run("decode-approval-"+field, func(t *testing.T) {
			wire := `{"prompt":[{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"approval","approved":false,"` + field + `":null}]}]}`
			_, err := decodeCallOptionsJSON([]byte(wire))
			require.Error(t, err)
		})
	}

	privateParts := []provider.ContentPart{
		{Type: provider.ContentPartTypeText, Text: "x", SourceType: provider.SourceTypeURL},
		{Type: provider.ContentPartTypeText, Text: "x", ID: "id"},
		{Type: provider.ContentPartTypeText, Text: "x", URL: "https://example.com"},
		{Type: provider.ContentPartTypeText, Text: "x", Title: "title"},
		{Type: provider.ContentPartTypeText, Text: "x", Signature: "signature"},
		{Type: provider.ContentPartTypeText, Text: "x", IsAutomatic: true},
	}
	for i, part := range privateParts {
		t.Run(fmt.Sprintf("encode-private-%d", i), func(t *testing.T) {
			_, err := EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(part)}})
			require.Error(t, err)
			_, err = EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleSystem, Content: []provider.ContentPart{part}}}})
			require.Error(t, err)
		})
	}

	approved := false
	privateApprovals := []provider.ContentPart{
		{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval", Approved: &approved, ToolCallID: "call"},
		{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval", Approved: &approved, ToolName: "tool"},
		{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval", Approved: &approved, ProviderExecuted: true},
	}
	for i, part := range privateApprovals {
		t.Run(fmt.Sprintf("encode-private-approval-%d", i), func(t *testing.T) {
			_, err := EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(part)}})
			require.Error(t, err)
		})
	}

	for _, wire := range []string{
		`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"","toolName":"tool","input":{}}]}]}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"","input":{}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"","toolName":"tool","output":{"type":"text","value":"ok"}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"","output":{"type":"text","value":"ok"}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"","approved":false}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"execution-denied","reason":null}}]}]}`,
	} {
		_, err := decodeCallOptionsJSON([]byte(wire))
		require.Error(t, err)
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

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
