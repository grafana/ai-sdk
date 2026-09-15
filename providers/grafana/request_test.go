package grafana

import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeRequest_Presence(t *testing.T) {
	zero := 0
	fzero := 0.0
	no := false
	input := provider.CallOptions{
		Prompt: []provider.Message{provider.NewSystemMessage(""), provider.UserText("")},
		Tools:  []provider.Tool{}, ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceAuto},
		MaxOutputTokens: &zero, Temperature: &fzero, TopP: &fzero, TopK: &zero, PresencePenalty: &fzero, FrequencyPenalty: &fzero,
		StopSequences: []string{}, ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatText}, Seed: &zero,
		Headers: map[string]string{"x-empty": ""}, ProviderOptions: provider.ProviderOptions{"empty": provider.RawProviderOption{Raw: json.RawMessage(`{}`)}, "opaque": provider.RawProviderOption{Raw: json.RawMessage(`{"nested":[null,false,0,"",[],{}]}`)}},
	}
	b, err := encodeRequest(input)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[{"role":"system","content":""},{"role":"user","content":[{"type":"text","text":""}]}],"tools":[],"toolChoice":{"type":"auto"},"maxOutputTokens":0,"temperature":0,"topP":0,"topK":0,"presencePenalty":0,"frequencyPenalty":0,"stopSequences":[],"responseFormat":{"type":"text"},"seed":0,"headers":{"x-empty":""},"providerOptions":{"empty":{},"opaque":{"nested":[null,false,0,"",[],{}]}}}`, string(b))
	b, err = encodeRequest(provider.CallOptions{})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(b))
	b, err = encodeRequest(provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "f", InputSchema: json.RawMessage(`{}`), InputExamples: []provider.InputExample{}, Strict: &no}}, Headers: map[string]string{}, ProviderOptions: provider.ProviderOptions{}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"tools":[{"type":"function","name":"f","inputSchema":{},"inputExamples":[],"strict":false}],"headers":{},"providerOptions":{}}`, string(b))
}

func TestEncodeRequest_DataArms(t *testing.T) {
	for _, tc := range []struct {
		name string
		data provider.DataContent
		want string
	}{
		{"bytes", provider.DataContent{Bytes: []byte{0, 1, 2}}, `{"type":"data","data":"AAEC"}`},
		{"empty bytes", provider.DataContent{Bytes: []byte{}}, `{"type":"data","data":""}`},
		{"base64", provider.Base64DataContent("AwQ="), `{"type":"data","data":"AwQ="}`},
		{"url", provider.DataContent{URL: "https://example.com/%"}, `{"type":"url","url":"https://example.com/%"}`},
		{"reference", provider.DataContent{Reference: json.RawMessage(`{}`)}, `{"type":"reference","reference":{}}`},
		{"text", provider.DataContent{Text: "text"}, `{"type":"text","text":"text"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := projectData(&tc.data)
			require.NoError(t, err)
			b, err := json.Marshal(value)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(b))
		})
	}
	for _, wire := range []string{`{"type":"text","text":""}`, `{"type":"url","url":""}`} {
		var data provider.DataContent
		require.NoError(t, json.Unmarshal([]byte(wire), &data))
		value, err := projectData(&data)
		require.NoError(t, err)
		b, err := json.Marshal(value)
		require.NoError(t, err)
		assert.JSONEq(t, wire, string(b))
	}
}

func TestEncodeRequest_ContentArms(t *testing.T) {
	no := false
	parts := []provider.ContentPart{
		provider.TextPart(""), provider.ReasoningPart(""), provider.FilePart("image/png", provider.DataContent{Bytes: []byte{0, 1, 2}}), provider.ReasoningFilePart("application/pdf", provider.DataContent{Bytes: []byte{3, 4}}), provider.CustomPart("p.custom"),
		provider.ToolCallPart("id", "tool", json.RawMessage(`{"nested":null}`)),
	}
	for _, kind := range []provider.ToolResultOutputType{provider.ToolOutputText, provider.ToolOutputJSON, provider.ToolOutputErrorText, provider.ToolOutputErrorJSON, provider.ToolOutputExecutionDenied, provider.ToolOutputContent} {
		output := &provider.ToolResultOutput{Type: kind}
		if kind == provider.ToolOutputContent {
			output.Content = []provider.ToolResultContentValue{{Type: provider.ToolContentText}, {Type: provider.ToolContentFile, Data: &provider.DataContent{Bytes: []byte{5, 6}}, MediaType: "image/png"}, {Type: provider.ToolContentCustom}}
		}
		parts = append(parts, provider.ToolResultPart("id", "tool", output))
	}
	b, err := encodeRequest(provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(parts...), provider.NewToolMessage(provider.ContentPart{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval", Approved: &no})}, Reasoning: provider.ReasoningHigh, IncludeRawChunks: true})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "high", got["reasoning"])
	assert.Equal(t, true, got["includeRawChunks"])
	assert.Contains(t, string(b), `"data":"BQY="`)
	assert.Contains(t, string(b), `"approved":false`)
	assert.Contains(t, string(b), `"value":null`)
	assert.NotContains(t, string(b), `"bytes"`)
}

func TestEncodeRequest_RejectsInvalid(t *testing.T) {
	nan := math.NaN()
	inf := math.Inf(1)
	for _, tc := range []struct {
		name string
		opts provider.CallOptions
	}{
		{"output-only approval request", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ToolApprovalRequestPart("approval", "id", false))}}},
		{"nil content", provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleUser}}}},
		{"reasoning reference", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ReasoningFilePart("text/plain", provider.DataContent{Reference: json.RawMessage(`{}`)}))}}},
		{"custom kind", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.CustomPart("unqualified"))}}},
		{"reserved reference", provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("text/plain", provider.DataContent{Reference: json.RawMessage(`{"type":"reserved"}`)}))}}},
		{"provider tool options", provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: "p.tool", Args: map[string]json.RawMessage{}, ProviderOptions: provider.ProviderOptions{}}}}},
		{"null input schema", provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, InputSchema: json.RawMessage(`null`)}}}},
		{"boolean response schema", provider.CallOptions{ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`true`)}}},
		{"nonobject provider options", provider.CallOptions{ProviderOptions: provider.ProviderOptions{"p": provider.RawProviderOption{Raw: json.RawMessage(`[]`)}}}},
		{"null provider options", provider.CallOptions{ProviderOptions: provider.ProviderOptions{"p": nil}}},
		{"nan", provider.CallOptions{Temperature: &nan}}, {"infinity", provider.CallOptions{TopP: &inf}},
		{"utf8", provider.CallOptions{Prompt: []provider.Message{provider.UserText(string([]byte{255}))}}},
		{"raw utf8", provider.CallOptions{ProviderOptions: provider.ProviderOptions{"p": provider.RawProviderOption{Raw: json.RawMessage{'"', 255, '"'}}}}},
		{"invalid raw", provider.CallOptions{ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{`)}}},
		{"unknown reasoning", provider.CallOptions{Reasoning: "provider-default"}},
		{"unknown role", provider.CallOptions{Prompt: []provider.Message{{Role: "alien"}}}},
		{"unknown part", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ContentPart{Type: "alien"})}}},
		{"inactive part", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ContentPart{Type: provider.ContentPartTypeText, ToolName: "lost"})}}},
		{"source unsupported", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ContentPart{Type: provider.ContentPartTypeSource})}}},
		{"system metadata lost", provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleSystem, Content: []provider.ContentPart{{Type: provider.ContentPartTypeText, ProviderOptions: provider.ProviderOptions{}}}}}}},
		{"conflicting data", provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", provider.DataContent{Bytes: []byte{}, URL: "https://example.test"}))}}},
		{"missing data", provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", provider.DataContent{}))}}},
		{"invalid tool", provider.CallOptions{Tools: []provider.Tool{{Type: "alien"}}}},
		{"inactive tool", provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeProvider, InputSchema: json.RawMessage(`{}`)}}}},
		{"invalid choice", provider.CallOptions{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceAuto, ToolName: "lost"}}},
		{"inactive format", provider.CallOptions{ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatText, Name: "lost"}}},
		{"header collision", provider.CallOptions{Headers: map[string]string{"x-test": "a", "X-Test": "b"}}},
		{"header newline", provider.CallOptions{Headers: map[string]string{"x-test": "a\nb"}}},
		{"header name", provider.CallOptions{Headers: map[string]string{"bad name": "x"}}},
		{"conflicting output", provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("id", "tool", &provider.ToolResultOutput{Type: provider.ToolOutputText, JSON: json.RawMessage(`null`)}))}}},
	} {
		t.Run(tc.name, func(t *testing.T) { b, err := encodeRequest(tc.opts); require.Error(t, err); assert.Nil(t, b) })
	}
}

func TestEncodeRequest_CallOptionsFieldWitness(t *testing.T) {
	fields := []string{"Prompt", "Tools", "ToolChoice", "MaxOutputTokens", "Temperature", "TopP", "TopK", "PresencePenalty", "FrequencyPenalty", "StopSequences", "ResponseFormat", "Seed", "Reasoning", "IncludeRawChunks", "Headers", "ProviderOptions"}
	typ := reflect.TypeFor[provider.CallOptions]()
	require.Equal(t, len(fields), typ.NumField(), "new CallOptions fields need explicit request mapping and parity classification")
	for i, name := range fields {
		assert.Equal(t, name, typ.Field(i).Name)
	}
}

func TestEncodeRequest_UnsafeInteger(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("32-bit integers are all JavaScript-safe")
	}
	large := int64(9007199254740992)
	value := int(large)
	_, err := encodeRequest(provider.CallOptions{Seed: &value})
	require.Error(t, err)
}
