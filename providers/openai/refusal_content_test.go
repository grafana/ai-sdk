package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/provider"
)

func TestConvertResponse_Refusal(t *testing.T) {
	const usage = `"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}`
	response := func(output string) string {
		return `{"id":"resp_1","created_at":1,"model":"gpt-4o","object":"response","status":"completed","output":[` + output + `],` + usage + `}`
	}
	const refusal = `{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"refusal","refusal":"I can't help."}]}`
	const functionCall = `{"type":"function_call","id":"fc_1","call_id":"call_1","name":"weather","arguments":"{}","status":"completed"}`

	t.Run("surfaces a refusal as text and finishes with content-filter", func(t *testing.T) {
		res := mustConvertResponse(t, decodeResponse(t, response(refusal)), buildResult{})
		require.Len(t, res.Content, 1)
		assert.Equal(t, provider.ContentText, res.Content[0].Type)
		assert.Equal(t, "I can't help.", res.Content[0].Text)
		assert.JSONEq(t, `{"itemId":"msg_1"}`, string(res.Content[0].ProviderMetadata["openai"]))
		assert.Equal(t, provider.FinishReasonContentFilter, res.FinishReason.Unified)
	})

	t.Run("keeps output text and refusal parts of one message", func(t *testing.T) {
		mixed := `{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Partial","annotations":[]},{"type":"refusal","refusal":"I can't continue."}]}`
		res := mustConvertResponse(t, decodeResponse(t, response(mixed)), buildResult{})
		require.Len(t, res.Content, 2)
		assert.Equal(t, "Partial", res.Content[0].Text)
		assert.Equal(t, "I can't continue.", res.Content[1].Text)
		assert.Equal(t, provider.FinishReasonContentFilter, res.FinishReason.Unified)
	})

	t.Run("a function call wins over a refusal", func(t *testing.T) {
		res := mustConvertResponse(t, decodeResponse(t, response(refusal+","+functionCall)), buildResult{})
		assert.Equal(t, provider.FinishReasonToolCalls, res.FinishReason.Unified)
	})
}

func TestStream_Refusal(t *testing.T) {
	const (
		created   = `{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","created_at":1,"model":"gpt-4o","object":"response","status":"in_progress","output":[]}}`
		added     = `{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`
		delta     = `{"type":"response.refusal.delta","sequence_number":2,"output_index":0,"content_index":0,"item_id":"msg_1","delta":"I can't"}`
		done      = `{"type":"response.refusal.done","sequence_number":3,"output_index":0,"content_index":0,"item_id":"msg_1","refusal":"I can't"}`
		itemDone  = `{"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"refusal","refusal":"I can't"}]}}`
		completed = `{"type":"response.completed","sequence_number":9,"response":{"id":"resp_1","created_at":1,"model":"gpt-4o","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`
		callAdded = `{"type":"response.output_item.added","sequence_number":5,"output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"weather","arguments":"","status":"in_progress"}}`
		callDone  = `{"type":"response.output_item.done","sequence_number":6,"output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"weather","arguments":"{}","status":"completed"}}`
	)

	t.Run("streams refusal deltas as text and finishes with content-filter", func(t *testing.T) {
		parts := collectParts(t, created, added, delta, done, itemDone, completed)
		assert.Equal(t, []provider.StreamPartType{
			provider.PartStreamStart, provider.PartResponseMeta, provider.PartTextStart,
			provider.PartTextDelta, provider.PartTextEnd, provider.PartFinish,
		}, partTypes(parts))
		assert.Equal(t, "msg_1", parts[3].ID)
		assert.Equal(t, "I can't", parts[3].Delta)
		finish := parts[len(parts)-1]
		require.NotNil(t, finish.FinishReason)
		assert.Equal(t, provider.FinishReasonContentFilter, finish.FinishReason.Unified)
	})

	t.Run("a streamed function call wins over a refusal", func(t *testing.T) {
		br := buildResult{functionTools: map[string]struct{}{"weather": {}}}
		parts := collectPartsWithBuildResult(t, br, created, added, delta, done, itemDone, callAdded, callDone, completed)
		finish := parts[len(parts)-1]
		require.NotNil(t, finish.FinishReason)
		assert.Equal(t, provider.FinishReasonToolCalls, finish.FinishReason.Unified)
		var calls int
		for _, part := range parts {
			if part.Type == provider.PartToolCall {
				calls++
				assert.True(t, json.Valid([]byte(part.Input)))
			}
		}
		assert.Equal(t, 1, calls)
	})
}
