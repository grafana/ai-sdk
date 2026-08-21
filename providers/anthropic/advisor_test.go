package anthropic

import (
	"encoding/json"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_AdvisorProviderTool(t *testing.T) {
	opts := provider.CallOptions{
		Prompt: []provider.Message{provider.NewUserMessage(provider.TextPart("Hello"))},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeProvider,
			ID:   "anthropic.advisor_20260301",
			Name: "consult",
			Args: map[string]json.RawMessage{
				"model":     json.RawMessage(`"claude-opus-4-7"`),
				"maxUses":   json.RawMessage(`5`),
				"maxTokens": json.RawMessage(`2048`),
				"caching":   json.RawMessage(`{"type":"ephemeral","ttl":"1h"}`),
			},
		}},
	}

	params, mapping, warnings, _, err := buildParams("claude-sonnet-4-6", opts, true)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, params.Tools, 1)
	advisor := params.Tools[0].OfAdvisorTool20260301
	require.NotNil(t, advisor)
	assert.Equal(t, sdk.Model("claude-opus-4-7"), advisor.Model)
	assert.Equal(t, int64(5), advisor.MaxUses.Value)
	assert.True(t, advisor.MaxUses.Valid())
	assert.Equal(t, int64(2048), advisor.MaxTokens.Value)
	assert.True(t, advisor.MaxTokens.Valid())
	assert.Equal(t, sdk.BetaCacheControlEphemeralTTLTTL1h, advisor.Caching.TTL)
	assert.Contains(t, params.Betas, sdk.AnthropicBetaAdvisorTool2026_03_01)
	assert.Equal(t, "advisor", mapping.toProviderToolName("consult"))
	assert.Equal(t, "consult", mapping.toCustomToolName("advisor"))
}

func TestBuildParams_AdvisorRoundTrip(t *testing.T) {
	parts := []provider.ContentPart{
		{
			Type:             provider.ContentPartTypeToolCall,
			ToolCallID:       "advisor-1",
			ToolName:         "consult",
			Input:            json.RawMessage(`{"ignored":true}`),
			ProviderExecuted: requestBoolPointer(true),
		},
		{
			Type:       provider.ContentPartTypeToolResult,
			ToolCallID: "advisor-1",
			ToolName:   "consult",
			Output: &provider.ToolResultOutput{
				Type: provider.ToolOutputJSON,
				JSON: json.RawMessage(`{"type":"advisor_redacted_result","encryptedContent":"ciphertext","stopReason":"end_turn"}`),
			},
		},
	}
	opts := provider.CallOptions{
		Prompt: []provider.Message{{Role: provider.RoleAssistant, Content: parts}},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeProvider,
			ID:   "anthropic.advisor_20260301",
			Name: "consult",
			Args: map[string]json.RawMessage{"model": json.RawMessage(`"claude-opus-4-7"`)},
		}},
	}

	params, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, params.Messages, 1)
	require.Len(t, params.Messages[0].Content, 2)

	call := params.Messages[0].Content[0].OfServerToolUse
	require.NotNil(t, call)
	assert.Equal(t, sdk.BetaServerToolUseBlockParamNameAdvisor, call.Name)
	assert.Equal(t, map[string]any{}, call.Input)

	result := params.Messages[0].Content[1].OfAdvisorToolResult
	require.NotNil(t, result)
	assert.Equal(t, "advisor-1", result.ToolUseID)
	require.NotNil(t, result.Content.OfRequestAdvisorRedactedResultBlock)
	assert.Equal(t, "ciphertext", result.Content.OfRequestAdvisorRedactedResultBlock.EncryptedContent)
	assert.Equal(t, "end_turn", result.Content.OfRequestAdvisorRedactedResultBlock.StopReason.Value)
	assert.True(t, result.Content.OfRequestAdvisorRedactedResultBlock.StopReason.Valid())
}

func TestConvertResponse_AdvisorResults(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
		isError  bool
	}{
		{name: "success", content: `{"type":"advisor_result","text":"use a bounded queue","stop_reason":"max_tokens"}`, expected: `{"type":"advisor_result","text":"use a bounded queue","stopReason":"max_tokens"}`},
		{name: "redacted", content: `{"type":"advisor_redacted_result","encrypted_content":"ciphertext","stop_reason":"end_turn"}`, expected: `{"type":"advisor_redacted_result","encryptedContent":"ciphertext","stopReason":"end_turn"}`},
		{name: "error", content: `{"type":"advisor_tool_result_error","error_code":"overloaded"}`, expected: `{"type":"advisor_tool_result_error","errorCode":"overloaded"}`, isError: true},
	}
	mapping := newToolNameMapping([]provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.advisor_20260301", Name: "consult"}})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := unmarshalMessage(t, `{
				"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6",
				"content":[
					{"type":"server_tool_use","id":"advisor-1","name":"advisor","input":{}},
					{"type":"advisor_tool_result","tool_use_id":"advisor-1","content":`+tc.content+`}
				],
				"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}
			}`)

			result, err := convertResponse(msg, mapping, false, nil, defaultGenerateID, "anthropic", false)
			require.NoError(t, err)
			require.Len(t, result.Content, 2)
			assert.Equal(t, provider.ContentToolCall, result.Content[0].Type)
			assert.Equal(t, "consult", result.Content[0].ToolName)
			assert.True(t, result.Content[0].ProviderExecuted)
			part := result.Content[1]
			assert.Equal(t, provider.ContentToolResult, part.Type)
			assert.Equal(t, "consult", part.ToolName)
			assert.Equal(t, tc.isError, part.IsError)
			assert.True(t, part.ProviderExecuted)
			assert.JSONEq(t, tc.expected, string(part.Result))
		})
	}
}

func TestStreamAdapter_AdvisorResults(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
		isError  bool
	}{
		{name: "success", content: `{"type":"advisor_result","text":"advice","stop_reason":"max_tokens"}`, expected: `{"type":"advisor_result","text":"advice","stopReason":"max_tokens"}`},
		{name: "redacted", content: `{"type":"advisor_redacted_result","encrypted_content":"ciphertext","stop_reason":"end_turn"}`, expected: `{"type":"advisor_redacted_result","encryptedContent":"ciphertext","stopReason":"end_turn"}`},
		{name: "error", content: `{"type":"advisor_tool_result_error","error_code":"unavailable"}`, expected: `{"type":"advisor_tool_result_error","errorCode":"unavailable"}`, isError: true},
	}
	mapping := newToolNameMapping([]provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.advisor_20260301", Name: "consult"}})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			events := []sdk.BetaRawMessageStreamEventUnion{
				unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"advisor-1","name":"advisor","input":{}}}`),
				unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
				unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":{"type":"advisor_tool_result","tool_use_id":"advisor-1","content":`+tc.content+`}}`),
			}
			parts := collectPartsWithMapping(events, mapping)
			require.Len(t, parts, 4)
			assert.Equal(t, provider.PartToolInputStart, parts[0].Type)
			assert.Equal(t, "consult", parts[0].ToolName)
			assert.Equal(t, provider.PartToolCall, parts[2].Type)
			assert.Equal(t, "{}", parts[2].Input)
			result := parts[3]
			assert.Equal(t, provider.PartToolResult, result.Type)
			assert.Equal(t, "consult", result.ToolName)
			assert.Equal(t, tc.isError, result.IsError)
			assert.True(t, result.ProviderExecuted)
			assert.JSONEq(t, tc.expected, string(result.Result))
		})
	}
}

func TestBuildParams_ReferencedAnthropicFiles(t *testing.T) {
	tests := []struct {
		name            string
		mediaType       string
		fileID          string
		providerOptions provider.ProviderOptions
		expected        string
	}{
		{name: "container upload", mediaType: "text/csv", fileID: "file-123", providerOptions: makeProviderOpts(`{"containerUpload":true}`), expected: `{"type":"container_upload","file_id":"file-123"}`},
		{name: "image", mediaType: "image/png", fileID: "file-123", expected: `{"type":"image","source":{"type":"file","file_id":"file-123"}}`},
		{name: "document", mediaType: "application/pdf", fileID: "file-123", expected: `{"type":"document","source":{"type":"file","file_id":"file-123"}}`},
		{name: "empty ID", mediaType: "application/pdf", expected: `{"type":"document","source":{"type":"file","file_id":""}}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			part := provider.FilePartWithFilename(tc.mediaType, provider.DataContent{Reference: json.RawMessage(`{"anthropic":"` + tc.fileID + `"}`)}, "report.pdf")
			part.ProviderOptions = tc.providerOptions
			params, _, warnings, _, err := buildParams("claude-3-haiku-20240307", provider.CallOptions{
				Prompt: []provider.Message{provider.NewUserMessage(part)},
			}, true)
			require.NoError(t, err)
			assert.Empty(t, warnings)
			assert.Contains(t, params.Betas, filesAPIBeta)
			require.Len(t, params.Messages[0].Content, 1)
			wire, err := json.Marshal(params.Messages[0].Content[0])
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(wire))
		})
	}
}

func TestBuildParams_ReferencedAnthropicFileRequiresProviderID(t *testing.T) {
	tests := []struct {
		name      string
		reference json.RawMessage
	}{
		{name: "different provider", reference: json.RawMessage(`{"openai":"file-123"}`)},
		{name: "null ID", reference: json.RawMessage(`{"anthropic":null}`)},
		{name: "malformed", reference: json.RawMessage(`{"anthropic":`)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			part := provider.FilePart("application/pdf", provider.DataContent{Reference: tc.reference})
			_, _, _, _, err := buildParams("claude-3-haiku-20240307", provider.CallOptions{
				Prompt: []provider.Message{provider.NewUserMessage(part)},
			}, true)
			if tc.name == "different provider" {
				require.ErrorContains(t, err, "no provider reference found for provider anthropic")
				require.ErrorContains(t, err, "available providers: openai")
			} else {
				require.ErrorContains(t, err, "invalid file data")
			}
		})
	}
}

func TestBuildParams_AdvisorProviderToolRejectsInvalidArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]json.RawMessage
	}{
		{name: "missing model", args: map[string]json.RawMessage{}},
		{name: "null model", args: map[string]json.RawMessage{"model": json.RawMessage(`null`)}},
		{name: "invalid model", args: map[string]json.RawMessage{"model": json.RawMessage(`1`)}},
		{name: "null max uses", args: map[string]json.RawMessage{"model": json.RawMessage(`"claude-opus-4-7"`), "maxUses": json.RawMessage(`null`)}},
		{name: "invalid max uses", args: map[string]json.RawMessage{"model": json.RawMessage(`"claude-opus-4-7"`), "maxUses": json.RawMessage(`"five"`)}},
		{name: "null max tokens", args: map[string]json.RawMessage{"model": json.RawMessage(`"claude-opus-4-7"`), "maxTokens": json.RawMessage(`null`)}},
		{name: "fractional max tokens", args: map[string]json.RawMessage{"model": json.RawMessage(`"claude-opus-4-7"`), "maxTokens": json.RawMessage(`2048.5`)}},
		{name: "max tokens below minimum", args: map[string]json.RawMessage{"model": json.RawMessage(`"claude-opus-4-7"`), "maxTokens": json.RawMessage(`1023`)}},
		{name: "invalid max tokens", args: map[string]json.RawMessage{"model": json.RawMessage(`"claude-opus-4-7"`), "maxTokens": json.RawMessage(`"many"`)}},
		{name: "null caching", args: map[string]json.RawMessage{"model": json.RawMessage(`"claude-opus-4-7"`), "caching": json.RawMessage(`null`)}},
		{name: "invalid caching type", args: map[string]json.RawMessage{"model": json.RawMessage(`"claude-opus-4-7"`), "caching": json.RawMessage(`{"type":"persistent","ttl":"5m"}`)}},
		{name: "invalid caching ttl", args: map[string]json.RawMessage{"model": json.RawMessage(`"claude-opus-4-7"`), "caching": json.RawMessage(`{"type":"ephemeral","ttl":"2h"}`)}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
				Prompt: []provider.Message{provider.NewUserMessage(provider.TextPart("Hello"))},
				Tools: []provider.Tool{{
					Type: provider.ToolTypeProvider,
					ID:   "anthropic.advisor_20260301",
					Name: "consult",
					Args: tc.args,
				}},
			}, true)
			require.ErrorContains(t, err, "invalid advisor tool arguments")
		})
	}
}

func TestBuildParams_AdvisorRoundTripRejectsInvalidResults(t *testing.T) {
	tests := []struct {
		name   string
		output json.RawMessage
	}{
		{name: "missing type", output: json.RawMessage(`{"text":"advice"}`)},
		{name: "null text", output: json.RawMessage(`{"type":"advisor_result","text":null}`)},
		{name: "missing encrypted content", output: json.RawMessage(`{"type":"advisor_redacted_result"}`)},
		{name: "null error code", output: json.RawMessage(`{"type":"advisor_tool_result_error","errorCode":null}`)},
		{name: "null stop reason", output: json.RawMessage(`{"type":"advisor_result","text":"advice","stopReason":null}`)},
		{name: "invalid stop reason", output: json.RawMessage(`{"type":"advisor_result","text":"advice","stopReason":1}`)},
		{name: "unknown type", output: json.RawMessage(`{"type":"future_advisor_result"}`)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(provider.ContentPart{
					Type:       provider.ContentPartTypeToolResult,
					ToolCallID: "advisor-1",
					ToolName:   "consult",
					Output: &provider.ToolResultOutput{
						Type: provider.ToolOutputJSON,
						JSON: tc.output,
					},
				})},
				Tools: []provider.Tool{{
					Type: provider.ToolTypeProvider,
					ID:   "anthropic.advisor_20260301",
					Name: "consult",
					Args: map[string]json.RawMessage{"model": json.RawMessage(`"claude-opus-4-7"`)},
				}},
			}, false)
			require.ErrorContains(t, err, "invalid advisor tool result")
		})
	}
}
