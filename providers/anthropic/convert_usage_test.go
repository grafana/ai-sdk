package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertAnthropicUsage(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantInput   int
		wantNoCache int
		wantOutput  int
	}{
		{
			name:        "cache tokens",
			raw:         `{"input_tokens":10,"output_tokens":20,"cache_creation_input_tokens":5,"cache_read_input_tokens":3}`,
			wantInput:   18,
			wantNoCache: 10,
			wantOutput:  20,
		},
		{
			name: "compaction excludes advisor iterations",
			raw: `{"input_tokens":20,"output_tokens":4,"cache_creation_input_tokens":5,"cache_read_input_tokens":3,"iterations":[` +
				`{"type":"compaction","input_tokens":100,"output_tokens":10},` +
				`{"type":"advisor_message","model":"claude-haiku-4-5","input_tokens":200,"output_tokens":20},` +
				`{"type":"message","input_tokens":30,"output_tokens":4}]}`,
			wantInput:   138,
			wantNoCache: 130,
			wantOutput:  14,
		},
		{
			name: "fallback uses top-level totals",
			raw: `{"input_tokens":40,"output_tokens":7,"cache_creation_input_tokens":2,"cache_read_input_tokens":1,"iterations":[` +
				`{"type":"message","model":"claude-opus-4-6","input_tokens":50,"output_tokens":0},` +
				`{"type":"fallback_message","model":"claude-sonnet-4-6","input_tokens":40,"output_tokens":7}]}`,
			wantInput:   43,
			wantNoCache: 40,
			wantOutput:  7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var usage anthropic.BetaUsage
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &usage))

			got := convertAnthropicUsage(anthropicUsage{
				inputTokens:              usage.InputTokens,
				outputTokens:             usage.OutputTokens,
				cacheCreationInputTokens: usage.CacheCreationInputTokens,
				cacheReadInputTokens:     usage.CacheReadInputTokens,
				iterations:               usage.Iterations,
				raw:                      json.RawMessage(usage.RawJSON()),
			})

			require.NotNil(t, got.InputTokens.Total)
			assert.Equal(t, tc.wantInput, *got.InputTokens.Total)
			require.NotNil(t, got.InputTokens.NoCache)
			assert.Equal(t, tc.wantNoCache, *got.InputTokens.NoCache)
			require.NotNil(t, got.InputTokens.CacheRead)
			assert.Equal(t, int(usage.CacheReadInputTokens), *got.InputTokens.CacheRead)
			require.NotNil(t, got.InputTokens.CacheWrite)
			assert.Equal(t, int(usage.CacheCreationInputTokens), *got.InputTokens.CacheWrite)
			require.NotNil(t, got.OutputTokens.Total)
			assert.Equal(t, tc.wantOutput, *got.OutputTokens.Total)
			assert.JSONEq(t, tc.raw, string(got.Raw))
		})
	}
}

func TestConvertAnthropicUsage_ThinkingTokens(t *testing.T) {
	var usage anthropic.BetaUsage
	require.NoError(t, json.Unmarshal([]byte(`{"input_tokens":10,"output_tokens":1}`), &usage))
	var first anthropic.BetaMessageDeltaUsage
	require.NoError(t, json.Unmarshal([]byte(`{"output_tokens":20,"output_tokens_details":{"thinking_tokens":7}}`), &first))

	adapter := &streamAdapter{}
	require.NoError(t, adapter.resetUsage(usage))
	require.NoError(t, adapter.updateUsage(first))
	got := convertAnthropicUsage(adapter.usage)
	require.NotNil(t, got.OutputTokens.Total)
	assert.Equal(t, 20, *got.OutputTokens.Total)
	require.NotNil(t, got.OutputTokens.Reasoning)
	assert.Equal(t, 7, *got.OutputTokens.Reasoning)
	require.NotNil(t, got.OutputTokens.Text)
	assert.Equal(t, 13, *got.OutputTokens.Text)

	var delta anthropic.BetaMessageDeltaUsage
	require.NoError(t, json.Unmarshal([]byte(`{"output_tokens":25,"output_tokens_details":{"thinking_tokens":9}}`), &delta))
	require.NoError(t, adapter.updateUsage(delta))
	got = convertAnthropicUsage(adapter.usage)
	assert.Equal(t, 25, *got.OutputTokens.Total)
	assert.Equal(t, 9, *got.OutputTokens.Reasoning)
	assert.Equal(t, 16, *got.OutputTokens.Text)
}

func TestStreamAdapter_MessageStartKeepsStreamLevelOutputUsage(t *testing.T) {
	var start anthropic.BetaUsage
	require.NoError(t, json.Unmarshal([]byte(`{"input_tokens":10,"output_tokens":20,"output_tokens_details":{"thinking_tokens":7}}`), &start))

	t.Run("output usage on message start is ignored", func(t *testing.T) {
		adapter := &streamAdapter{}
		require.NoError(t, adapter.resetUsage(start))
		got := convertAnthropicUsage(adapter.usage)
		require.NotNil(t, got.InputTokens.Total)
		assert.Equal(t, 10, *got.InputTokens.Total)
		require.NotNil(t, got.OutputTokens.Total)
		assert.Equal(t, 0, *got.OutputTokens.Total)
		assert.Nil(t, got.OutputTokens.Reasoning)
	})

	t.Run("a later message start keeps the previous output usage", func(t *testing.T) {
		adapter := &streamAdapter{}
		var delta anthropic.BetaMessageDeltaUsage
		require.NoError(t, json.Unmarshal([]byte(`{"output_tokens":25,"output_tokens_details":{"thinking_tokens":9}}`), &delta))
		require.NoError(t, adapter.updateUsage(delta))
		require.NoError(t, adapter.resetUsage(start))
		got := convertAnthropicUsage(adapter.usage)
		assert.Equal(t, 25, *got.OutputTokens.Total)
		assert.Equal(t, 9, *got.OutputTokens.Reasoning)
		assert.Equal(t, 10, *got.InputTokens.Total)
	})
}

func TestConvertAnthropicUsage_MissingOrNullThinkingTokensRemainNil(t *testing.T) {
	tests := []struct {
		name  string
		usage string
	}{
		{name: "parent omitted", usage: `{"input_tokens":10,"output_tokens":20}`},
		{name: "parent null", usage: `{"input_tokens":10,"output_tokens":20,"output_tokens_details":null}`},
		{name: "child omitted", usage: `{"input_tokens":10,"output_tokens":20,"output_tokens_details":{}}`},
		{name: "child null", usage: `{"input_tokens":10,"output_tokens":20,"output_tokens_details":{"thinking_tokens":null}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var usage anthropic.BetaMessageDeltaUsage
			require.NoError(t, json.Unmarshal([]byte(tc.usage), &usage))
			adapter := &streamAdapter{}
			require.NoError(t, adapter.updateUsage(usage))
			got := convertAnthropicUsage(adapter.usage)
			assert.Nil(t, got.OutputTokens.Reasoning)
			assert.Nil(t, got.OutputTokens.Text)
		})
	}

	t.Run("explicit zero is retained", func(t *testing.T) {
		var usage anthropic.BetaMessageDeltaUsage
		require.NoError(t, json.Unmarshal([]byte(`{"input_tokens":10,"output_tokens":20,"output_tokens_details":{"thinking_tokens":0}}`), &usage))
		adapter := &streamAdapter{}
		require.NoError(t, adapter.updateUsage(usage))
		got := convertAnthropicUsage(adapter.usage)
		require.NotNil(t, got.OutputTokens.Reasoning)
		assert.Equal(t, 0, *got.OutputTokens.Reasoning)
		require.NotNil(t, got.OutputTokens.Text)
		assert.Equal(t, 20, *got.OutputTokens.Text)
	})
}

func TestConvertAnthropicUsage_DeltaThinkingTokenPresence(t *testing.T) {
	var start anthropic.BetaUsage
	require.NoError(t, json.Unmarshal([]byte(`{"input_tokens":10,"output_tokens":1}`), &start))
	var initial anthropic.BetaMessageDeltaUsage
	require.NoError(t, json.Unmarshal([]byte(`{"output_tokens":20,"output_tokens_details":{"thinking_tokens":7}}`), &initial))
	newAdapter := func(t *testing.T) *streamAdapter {
		t.Helper()
		adapter := &streamAdapter{}
		require.NoError(t, adapter.resetUsage(start))
		require.NoError(t, adapter.updateUsage(initial))
		return adapter
	}

	t.Run("omitted parent preserves the previous count", func(t *testing.T) {
		adapter := newAdapter(t)
		var delta anthropic.BetaMessageDeltaUsage
		require.NoError(t, json.Unmarshal([]byte(`{"output_tokens":25}`), &delta))
		require.NoError(t, adapter.updateUsage(delta))
		got := convertAnthropicUsage(adapter.usage)
		assert.Equal(t, 7, *got.OutputTokens.Reasoning)
		assert.Equal(t, 18, *got.OutputTokens.Text)
	})

	t.Run("parent null preserves the previous count", func(t *testing.T) {
		adapter := newAdapter(t)
		var delta anthropic.BetaMessageDeltaUsage
		require.NoError(t, json.Unmarshal([]byte(`{"output_tokens":25,"output_tokens_details":null}`), &delta))
		require.NoError(t, adapter.updateUsage(delta))
		got := convertAnthropicUsage(adapter.usage)
		assert.Equal(t, 7, *got.OutputTokens.Reasoning)
		assert.Equal(t, 18, *got.OutputTokens.Text)
	})

	for _, tc := range []struct {
		name  string
		delta string
	}{
		{name: "child omitted clears", delta: `{"output_tokens":25,"output_tokens_details":{}}`},
		{name: "child null clears", delta: `{"output_tokens":25,"output_tokens_details":{"thinking_tokens":null}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newAdapter(t)
			var delta anthropic.BetaMessageDeltaUsage
			require.NoError(t, json.Unmarshal([]byte(tc.delta), &delta))
			require.NoError(t, adapter.updateUsage(delta))
			got := convertAnthropicUsage(adapter.usage)
			assert.Nil(t, got.OutputTokens.Reasoning)
			assert.Nil(t, got.OutputTokens.Text)
		})
	}
}

func TestConvertResponse_ThinkingTokenUsage(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id":"msg_1",
		"type":"message",
		"role":"assistant",
		"model":"claude-opus-5",
		"content":[{"type":"text","text":"done"}],
		"stop_reason":"end_turn",
		"usage":{"input_tokens":10,"output_tokens":20,"output_tokens_details":{"thinking_tokens":7}}
	}`)

	result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
	require.NoError(t, err)
	require.NotNil(t, result.Usage.OutputTokens.Total)
	assert.Equal(t, 20, *result.Usage.OutputTokens.Total)
	require.NotNil(t, result.Usage.OutputTokens.Reasoning)
	assert.Equal(t, 7, *result.Usage.OutputTokens.Reasoning)
	require.NotNil(t, result.Usage.OutputTokens.Text)
	assert.Equal(t, 13, *result.Usage.OutputTokens.Text)
}

func TestConvertResponse_NullThinkingTokenUsageRemainsNil(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id":"msg_1",
		"type":"message",
		"role":"assistant",
		"model":"claude-opus-5",
		"content":[{"type":"text","text":"done"}],
		"stop_reason":"end_turn",
		"usage":{"input_tokens":10,"output_tokens":20,"output_tokens_details":{"thinking_tokens":null}}
	}`)

	result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
	require.NoError(t, err)
	assert.Nil(t, result.Usage.OutputTokens.Reasoning)
	assert.Nil(t, result.Usage.OutputTokens.Text)
}

func TestConvertResponse_Usage(t *testing.T) {
	msg := unmarshalMessage(t, `{
		"id":"msg_1",
		"type":"message",
		"role":"assistant",
		"model":"claude-sonnet-4-6",
		"content":[{"type":"text","text":"done"}],
		"stop_reason":"end_turn",
		"usage":{
			"input_tokens":20,
			"output_tokens":4,
			"cache_creation_input_tokens":5,
			"cache_read_input_tokens":3,
			"iterations":[
				{"type":"compaction","input_tokens":100,"output_tokens":10},
				{"type":"message","input_tokens":30,"output_tokens":4}
			]
		}
	}`)

	result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
	require.NoError(t, err)
	require.NotNil(t, result.Usage.InputTokens.Total)
	assert.Equal(t, 138, *result.Usage.InputTokens.Total)
	require.NotNil(t, result.Usage.OutputTokens.Total)
	assert.Equal(t, 14, *result.Usage.OutputTokens.Total)
	assert.JSONEq(t, msg.Usage.RawJSON(), string(result.Usage.Raw))
}

func TestFilterUsageIterations(t *testing.T) {
	t.Run("keeps the fields the upstream schema declares", func(t *testing.T) {
		filtered, err := filterUsageIterations(json.RawMessage(`[{"type":"message","model":"m","input_tokens":1,"output_tokens":2,"cache_creation_input_tokens":3,"cache_read_input_tokens":4,"cache_creation":{"ephemeral_5m_input_tokens":0},"future":true}]`))
		require.NoError(t, err)
		assert.JSONEq(t, `[{"type":"message","model":"m","input_tokens":1,"output_tokens":2,"cache_creation_input_tokens":3,"cache_read_input_tokens":4}]`, string(filtered))
	})

	t.Run("null passes through", func(t *testing.T) {
		filtered, err := filterUsageIterations(json.RawMessage(`null`))
		require.NoError(t, err)
		assert.Equal(t, "null", string(filtered))
	})

	t.Run("merged raw usage is filtered", func(t *testing.T) {
		adapter := &streamAdapter{}
		var delta anthropic.BetaMessageDeltaUsage
		require.NoError(t, json.Unmarshal([]byte(`{"output_tokens":5,"iterations":[{"type":"message","input_tokens":1,"output_tokens":5,"cache_creation":{"ephemeral_1h_input_tokens":0}}]}`), &delta))
		require.NoError(t, adapter.updateUsage(delta))
		assert.JSONEq(t, `{"output_tokens":5,"iterations":[{"type":"message","input_tokens":1,"output_tokens":5}]}`, string(adapter.usage.raw))
	})
}
