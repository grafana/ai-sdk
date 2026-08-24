package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseResponse_TextOnly(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [{"text": "hello world"}]}},
		"stopReason": "end_turn",
		"usage": {"inputTokens": 10, "outputTokens": 5}
	}`)
	result, err := parseResponse(body, nil, testAnthropicModel, requestMeta{}, defaultGenerateID)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, provider.ContentText, result.Content[0].Type)
	assert.Equal(t, "hello world", result.Content[0].Text)
	assert.Equal(t, provider.FinishReasonStop, result.FinishReason.Unified)
	assert.Equal(t, "end_turn", result.FinishReason.Raw)
	assert.Equal(t, 10, *result.Usage.InputTokens.NoCache)
	assert.Equal(t, 5, *result.Usage.OutputTokens.Total)
	require.Contains(t, result.ProviderMetadata, "bedrock")
	var metadata map[string]any
	require.NoError(t, json.Unmarshal(result.ProviderMetadata["bedrock"], &metadata))
	require.Contains(t, metadata, "stopSequence")
	assert.Nil(t, metadata["stopSequence"])
}

func TestConvertUsagePreservesExplicitNullRawFields(t *testing.T) {
	var usage converseUsage
	require.NoError(t, json.Unmarshal([]byte(`{
		"inputTokens":10,
		"outputTokens":5,
		"cacheReadInputTokens":null,
		"cacheWriteInputTokens":null
	}`), &usage))

	got := convertUsage(&usage)
	require.JSONEq(t, `{
		"inputTokens":10,
		"outputTokens":5,
		"cacheReadInputTokens":null,
		"cacheWriteInputTokens":null
	}`, string(got.Raw))
}

func TestParseResponse_JSONInstructionExtractsObject(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [
			{"text": "Here is the result: "},
			{"text": "{\"status\":{\"value\":\"ok } still string\"}} trailing text"}
		]}},
		"stopReason": "end_turn",
		"usage": {"inputTokens": 10, "outputTokens": 5}
	}`)
	result, err := parseResponse(body, nil, testAnthropicModel, requestMeta{usesJSONInstruction: true}, defaultGenerateID)
	require.NoError(t, err)
	require.Len(t, result.Content, 2)
	assert.Empty(t, result.Content[0].Text)
	assert.JSONEq(t, `{"status":{"value":"ok } still string"}}`, result.Content[1].Text)
}

func TestParseResponse_ToolCall(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [
			{"toolUse": {"toolUseId": "call-1", "name": "weather", "input": {"city":"Berlin"}}}
		]}},
		"stopReason": "tool_use",
		"usage": {"inputTokens": 10, "outputTokens": 20}
	}`)
	result, err := parseResponse(body, nil, testAnthropicModel, requestMeta{}, defaultGenerateID)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, provider.ContentToolCall, result.Content[0].Type)
	assert.Equal(t, "call-1", result.Content[0].ToolCallID)
	assert.Equal(t, "weather", result.Content[0].ToolName)
	assert.JSONEq(t, `{"city":"Berlin"}`, string(result.Content[0].Input))
	assert.Equal(t, provider.FinishReasonToolCalls, result.FinishReason.Unified)
}

func TestParseResponse_ReasoningWithSignature(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [
			{"reasoningContent": {"reasoningText": {"text": "thinking...", "signature": "sig-xyz"}}}
		]}},
		"stopReason": "end_turn",
		"usage": {"inputTokens": 10, "outputTokens": 0}
	}`)
	result, err := parseResponse(body, nil, testAnthropicModel, requestMeta{}, defaultGenerateID)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, provider.ContentReasoning, result.Content[0].Type)
	assert.Equal(t, "thinking...", result.Content[0].Text)
	require.NotNil(t, result.Content[0].ProviderMetadata)
	require.Contains(t, result.Content[0].ProviderMetadata, "amazonBedrock")
	raw := result.Content[0].ProviderMetadata["amazonBedrock"]
	var meta ReasoningMetadata
	require.NoError(t, json.Unmarshal(raw, &meta))
	assert.Equal(t, "sig-xyz", meta.Signature)
}

func TestParseResponse_RedactedReasoning(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [
			{"reasoningContent": {"redactedReasoning": {"data": "redacted-blob"}}}
		]}},
		"stopReason": "end_turn",
		"usage": {"inputTokens": 5, "outputTokens": 0}
	}`)
	result, err := parseResponse(body, nil, testAnthropicModel, requestMeta{}, defaultGenerateID)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, provider.ContentReasoning, result.Content[0].Type)
	assert.Equal(t, "", result.Content[0].Text)
	raw := result.Content[0].ProviderMetadata["amazonBedrock"]
	var meta ReasoningMetadata
	require.NoError(t, json.Unmarshal(raw, &meta))
	assert.Equal(t, "redacted-blob", meta.RedactedData)
}

func TestParseResponse_RedactedContent(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [
			{"reasoningContent": {"redactedContent": "encrypted-reasoning"}}
		]}},
		"stopReason": "end_turn",
		"usage": {"inputTokens": 5, "outputTokens": 0}
	}`)
	result, err := parseResponse(body, nil, testAnthropicModel, requestMeta{}, defaultGenerateID)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, provider.ContentReasoning, result.Content[0].Type)
	var meta ReasoningMetadata
	require.NoError(t, json.Unmarshal(result.Content[0].ProviderMetadata["amazonBedrock"], &meta))
	assert.Equal(t, "encrypted-reasoning", meta.RedactedContent)
}

func TestParseResponse_UsageWithCache(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [{"text": "hi"}]}},
		"stopReason": "end_turn",
		"usage": {"inputTokens": 10, "outputTokens": 20, "cacheReadInputTokens": 3, "cacheWriteInputTokens": 5}
	}`)
	result, err := parseResponse(body, nil, testAnthropicModel, requestMeta{}, defaultGenerateID)
	require.NoError(t, err)
	assert.Equal(t, 18, *result.Usage.InputTokens.Total)
	assert.Equal(t, 10, *result.Usage.InputTokens.NoCache)
	assert.Equal(t, 3, *result.Usage.InputTokens.CacheRead)
	assert.Equal(t, 5, *result.Usage.InputTokens.CacheWrite)
}

func TestMapFinishReason(t *testing.T) {
	cases := []struct {
		raw      string
		jsonTool bool
		want     provider.UnifiedFinishReason
	}{
		{"end_turn", false, provider.FinishReasonStop},
		{"stop_sequence", false, provider.FinishReasonStop},
		{"max_tokens", false, provider.FinishReasonLength},
		{"content_filtered", false, provider.FinishReasonContentFilter},
		{"guardrail_intervened", false, provider.FinishReasonContentFilter},
		{"tool_use", false, provider.FinishReasonToolCalls},
		{"tool_use", true, provider.FinishReasonStop},
		{"unknown_reason", false, provider.FinishReasonOther},
	}
	for _, tc := range cases {
		got := mapFinishReason(tc.raw, tc.jsonTool)
		assert.Equal(t, tc.want, got.Unified, "stopReason=%s jsonTool=%v", tc.raw, tc.jsonTool)
		assert.Equal(t, tc.raw, got.Raw)
	}
}

func TestParseResponse_JSONResponseToolCollapse(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [
			{"toolUse": {"toolUseId": "call-1", "name": "json", "input": {"url":"https:\/\/x","n":1e+2,"small":1e-7,"threshold":1e-6,"html":"<tag>","2":"two","1":"one","dup":"first","orderB":1,"dup":"last","orderA":2}}}
		]}},
		"stopReason": "tool_use",
		"usage": {"inputTokens": 10, "outputTokens": 5}
	}`)
	result, err := parseResponse(body, nil, testAnthropicModel,
		requestMeta{usesJSONResponseTool: true}, defaultGenerateID)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, provider.ContentText, result.Content[0].Type)
	assert.Equal(t, `{"1":"one","2":"two","url":"https://x","n":100,"small":1e-7,"threshold":0.000001,"html":"<tag>","dup":"last","orderB":1,"orderA":2}`, result.Content[0].Text)
	// FinishReason flipped to "stop" because the JSON tool acted as the final answer.
	assert.Equal(t, provider.FinishReasonStop, result.FinishReason.Unified)
	// ProviderMetadata should record isJsonResponseFromTool.
	require.NotNil(t, result.ProviderMetadata)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(result.ProviderMetadata["amazonBedrock"], &payload))
	assert.Equal(t, true, payload["isJsonResponseFromTool"])
}

func TestParseResponse_ResponseHeaders(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [{"text": "ok"}]}},
		"stopReason": "end_turn",
		"usage": {"inputTokens": 1, "outputTokens": 1}
	}`)
	headers := map[string][]string{
		"X-Amzn-Requestid": {"req-123"},
		"Date":             {"Mon, 02 Jan 2006 15:04:05 GMT"},
		"Other":            {"x"},
	}
	result, err := parseResponse(body, headers, "anthropic.claude-3-haiku-20240307-v1:0", requestMeta{}, defaultGenerateID)
	require.NoError(t, err)
	require.NotNil(t, result.Response)
	assert.Equal(t, "req-123", result.Response.ID)
	assert.Equal(t, "anthropic.claude-3-haiku-20240307-v1:0", result.Response.ModelID)
	assert.Equal(t, providerName, result.Response.Provider, "served provider tagged on response metadata")
	assert.False(t, result.Response.Timestamp.IsZero(), "timestamp should be parsed from date header")
}

func TestRunStream_ResponseMetadataIncludesProvider(t *testing.T) {
	body := encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hi"}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"messageStop":{"stopReason":"end_turn"}}`,
		`{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`,
	)
	parts := drainStreamRawWithHeaders(t, body, requestMeta{}, map[string][]string{
		"X-Amzn-Requestid": {"req-1"},
	})
	metaIdx := findParts(parts, provider.PartResponseMeta)
	require.Len(t, metaIdx, 1)
	assert.Equal(t, providerName, parts[metaIdx[0]].Provider)
}

func TestParseResponse_MistralToolCallNormalized(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [
			{"toolUse": {"toolUseId": "tooluse_bpe71yCfRu2b5i-nKGDr5g", "name": "weather", "input": {}}}
		]}},
		"stopReason": "tool_use",
		"usage": {"inputTokens": 1, "outputTokens": 1}
	}`)
	result, err := parseResponse(body, nil, testMistralModel, requestMeta{isMistral: true}, defaultGenerateID)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "toolusebp", result.Content[0].ToolCallID)
}

func TestParseResponse_ErrorOnInvalidJSON(t *testing.T) {
	_, err := parseResponse([]byte("not json"), nil, testAnthropicModel, requestMeta{}, defaultGenerateID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding response")
}

func TestParseResponse_ErrorOnMissingOutput(t *testing.T) {
	_, err := parseResponse([]byte(`{"stopReason":"end_turn"}`), nil, testAnthropicModel, requestMeta{}, defaultGenerateID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing output.message")
}

func TestParseResponse_StopSequenceAlwaysPresentWhenMetadata(t *testing.T) {
	// When a metadata payload exists (here: isJsonResponseFromTool), upstream
	// always includes stopSequence, defaulting to null when absent.
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [
			{"toolUse": {"toolUseId": "call-1", "name": "json", "input": {"foo":"bar"}}}
		]}},
		"stopReason": "tool_use",
		"usage": {"inputTokens": 10, "outputTokens": 5}
	}`)
	result, err := parseResponse(body, nil, testAnthropicModel,
		requestMeta{usesJSONResponseTool: true}, defaultGenerateID)
	require.NoError(t, err)
	require.NotNil(t, result.ProviderMetadata)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(result.ProviderMetadata["amazonBedrock"], &payload))
	require.Contains(t, payload, "stopSequence", "stopSequence must always be present in metadata")
	assert.Nil(t, payload["stopSequence"], "stopSequence defaults to null when absent")
}

func TestParseResponse_StopSequenceValuePropagated(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [{"text": "ok"}]}},
		"stopReason": "stop_sequence",
		"usage": {"inputTokens": 1, "outputTokens": 1},
		"additionalModelResponseFields": {"delta": {"stop_sequence": "END"}}
	}`)
	result, err := parseResponse(body, nil, testAnthropicModel, requestMeta{}, defaultGenerateID)
	require.NoError(t, err)
	require.NotNil(t, result.ProviderMetadata)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(result.ProviderMetadata["amazonBedrock"], &payload))
	assert.Equal(t, "END", payload["stopSequence"])
}

func TestParseResponse_ToolCallIDFallbackWhenMissing(t *testing.T) {
	body := []byte(`{
		"output": {"message": {"role": "assistant", "content": [
			{"toolUse": {"toolUseId": "", "name": "", "input": {}}}
		]}},
		"stopReason": "tool_use",
		"usage": {"inputTokens": 1, "outputTokens": 1}
	}`)
	gen := func() string { return "GENERATED" }
	result, err := parseResponse(body, nil, testAnthropicModel, requestMeta{}, gen)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "GENERATED", result.Content[0].ToolCallID)
	assert.Equal(t, "tool-GENERATED", result.Content[0].ToolName)
}
