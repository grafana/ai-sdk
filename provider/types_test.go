package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinishReasonConstants(t *testing.T) {
	cases := []struct {
		reason UnifiedFinishReason
		want   string
	}{
		{FinishReasonStop, "stop"},
		{FinishReasonLength, "length"},
		{FinishReasonContentFilter, "content-filter"},
		{FinishReasonToolCalls, "tool-calls"},
		{FinishReasonError, "error"},
		{FinishReasonOther, "other"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, string(tc.reason))
	}
}

func TestFinishReasonStruct(t *testing.T) {
	fr := FinishReason{Unified: FinishReasonStop, Raw: "end_turn"}
	data, err := json.Marshal(fr)
	require.NoError(t, err)
	assert.JSONEq(t, `{"unified":"stop","raw":"end_turn"}`, string(data))

	frNoRaw := FinishReason{Unified: FinishReasonStop}
	data, err = json.Marshal(frNoRaw)
	require.NoError(t, err)
	assert.JSONEq(t, `{"unified":"stop"}`, string(data))
}

func TestToolResultOutputTypeConstants(t *testing.T) {
	cases := []struct {
		typ  ToolResultOutputType
		want string
	}{
		{ToolOutputText, "text"},
		{ToolOutputJSON, "json"},
		{ToolOutputContent, "content"},
		{ToolOutputExecutionDenied, "execution-denied"},
		{ToolOutputErrorText, "error-text"},
		{ToolOutputErrorJSON, "error-json"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, string(tc.typ))
	}
}

func TestUsageZeroValue(t *testing.T) {
	var u Usage
	assert.Nil(t, u.InputTokens.Total)
	assert.Nil(t, u.OutputTokens.Total)
	assert.Nil(t, u.Raw)
}

func TestUsageJSONSerialization(t *testing.T) {
	total := 100
	cacheRead := 30
	u := Usage{
		InputTokens:  InputTokenUsage{Total: &total, CacheRead: &cacheRead},
		OutputTokens: OutputTokenUsage{Total: &total},
	}
	data, err := json.Marshal(u)
	require.NoError(t, err)
	assert.JSONEq(t, `{"inputTokens":{"total":100,"cacheRead":30},"outputTokens":{"total":100}}`, string(data))

	var decoded Usage
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded.InputTokens.Total)
	assert.Equal(t, 100, *decoded.InputTokens.Total)
	require.NotNil(t, decoded.InputTokens.CacheRead)
	assert.Equal(t, 30, *decoded.InputTokens.CacheRead)
	assert.Nil(t, decoded.InputTokens.CacheWrite)
}

func TestToolResultContentValue_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		val  ToolResultContentValue
		want string
	}{
		{
			name: "file data",
			val:  ToolResultContentValue{Type: ToolContentFile, Data: &DataContent{Base64: "base64data"}, MediaType: "application/pdf", Filename: "report.pdf"},
			want: `{"type":"file","data":{"type":"data","data":"base64data"},"mediaType":"application/pdf","filename":"report.pdf"}`,
		},
		{
			name: "file URL",
			val:  ToolResultContentValue{Type: ToolContentFile, Data: &DataContent{URL: "https://example.com/file.pdf"}, MediaType: "application/pdf"},
			want: `{"type":"file","data":{"type":"url","url":"https://example.com/file.pdf"},"mediaType":"application/pdf"}`,
		},
		{
			name: "file reference",
			val:  ToolResultContentValue{Type: ToolContentFile, Data: &DataContent{Reference: json.RawMessage(`{"openai":"file-abc123"}`)}, MediaType: "application/pdf"},
			want: `{"type":"file","data":{"type":"reference","reference":{"openai":"file-abc123"}},"mediaType":"application/pdf"}`,
		},
		{
			name: "file text",
			val:  ToolResultContentValue{Type: ToolContentFile, Data: &DataContent{Text: "document"}, MediaType: "text/plain"},
			want: `{"type":"file","data":{"type":"text","text":"document"},"mediaType":"text/plain"}`,
		},
		{
			name: "custom",
			val:  ToolResultContentValue{Type: ToolContentCustom},
			want: `{"type":"custom"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.val)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(data))

			var decoded ToolResultContentValue
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, tc.val, decoded)
		})
	}
}

func TestToolResultOutput(t *testing.T) {
	t.Run("text variant has zero-valued non-text fields", func(t *testing.T) {
		out := ToolResultOutput{Type: ToolOutputText, Text: "result data"}
		assert.Nil(t, out.JSON)
		assert.Nil(t, out.Content)
		assert.Empty(t, out.Reason)
	})

	t.Run("json variant has zero-valued non-json fields", func(t *testing.T) {
		out := ToolResultOutput{Type: ToolOutputJSON, JSON: json.RawMessage(`{"key":"value"}`)}
		assert.Empty(t, out.Text)
		assert.Nil(t, out.Content)
		assert.Empty(t, out.Reason)
	})
}

func TestToolResultOutput_ProviderOptionsRoundTrip(t *testing.T) {
	out := ToolResultOutput{
		Type: ToolOutputText,
		Text: "ok",
		ProviderOptions: ProviderOptions{
			"anthropic": RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"cache":"ephemeral"}`)},
		},
	}
	data, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"providerOptions"`)

	var decoded ToolResultOutput
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, out.Type, decoded.Type)
	assert.Equal(t, out.Text, decoded.Text)
	require.NotNil(t, decoded.ProviderOptions)
	raw, ok := decoded.ProviderOptions["anthropic"].(RawProviderOption)
	require.True(t, ok)
	assert.JSONEq(t, `{"cache":"ephemeral"}`, string(raw.Raw))
}

func TestToolResultContentValue_ProviderOptionsRoundTrip(t *testing.T) {
	val := ToolResultContentValue{
		Type: ToolContentText,
		Text: "hi",
		ProviderOptions: ProviderOptions{
			"openai": RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"x":1}`)},
		},
	}
	data, err := json.Marshal(val)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"providerOptions"`)

	var decoded ToolResultContentValue
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, val.Type, decoded.Type)
	require.NotNil(t, decoded.ProviderOptions)
	raw, ok := decoded.ProviderOptions["openai"].(RawProviderOption)
	require.True(t, ok)
	assert.JSONEq(t, `{"x":1}`, string(raw.Raw))
}
