package agentobservability

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRecorderForStreamTest() *StreamRecorder {
	return NewStreamRecorder(agento11y.GenerationStart{
		Model: agento11y.ModelRef{Provider: "anthropic", Name: "claude"},
	}, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
}

func TestStreamRecorder_TextOnlyStream(t *testing.T) {
	r := newRecorderForStreamTest()
	r.Observe(provider.StreamPart{Type: provider.PartTextStart, ID: "t0"})
	r.Observe(provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: "Hello, "})
	r.Observe(provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: "world"})
	r.Observe(provider.StreamPart{Type: provider.PartTextEnd, ID: "t0"})
	fr := provider.FinishReason{Unified: provider.FinishReasonStop}
	out := 5
	r.Observe(provider.StreamPart{
		Type:         provider.PartFinish,
		FinishReason: &fr,
		Usage: &provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intPtr(3)},
			OutputTokens: provider.OutputTokenUsage{Total: &out},
		},
	})

	gen := r.Generation()
	require.Len(t, gen.Output, 1)
	require.Len(t, gen.Output[0].Parts, 1)
	assert.Equal(t, agento11y.PartKindText, gen.Output[0].Parts[0].Kind)
	assert.Equal(t, "Hello, world", gen.Output[0].Parts[0].Text)
	assert.Equal(t, int64(3), gen.Usage.InputTokens)
	assert.Equal(t, int64(5), gen.Usage.OutputTokens)
	assert.Equal(t, "end_turn", gen.StopReason)
}

func TestStreamRecorder_UsageAggregatesEveryPart(t *testing.T) {
	r := newRecorderForStreamTest()
	inputTotal, inputNoCache, cacheRead, cacheWrite := 120, 80, 30, 10
	outputTotal, outputText, outputReasoning := 50, 30, 20
	provisionalInput, provisionalCacheRead, provisionalCacheWrite := 100, 20, 5
	provisionalOutput, provisionalText, provisionalReasoning := 45, 25, 15
	r.Observe(provider.StreamPart{Type: provider.PartResponseMeta, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{
		Total: &inputTotal, NoCache: &inputNoCache, CacheRead: &cacheRead, CacheWrite: &cacheWrite,
	}}})
	r.Observe(provider.StreamPart{Type: provider.PartTextDelta, Usage: &provider.Usage{OutputTokens: provider.OutputTokenUsage{
		Total: &outputTotal, Text: &outputText, Reasoning: &outputReasoning,
	}}})
	r.Observe(provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{
		InputTokens: provider.InputTokenUsage{
			Total: &provisionalInput, CacheRead: &provisionalCacheRead, CacheWrite: &provisionalCacheWrite,
		},
		OutputTokens: provider.OutputTokenUsage{
			Total: &provisionalOutput, Text: &provisionalText, Reasoning: &provisionalReasoning,
		},
	}})

	usage := r.Generation().Usage
	assert.Equal(t, int64(inputTotal), usage.InputTokens)
	assert.Equal(t, int64(outputTotal), usage.OutputTokens)
	assert.Equal(t, int64(inputTotal+outputTotal), usage.TotalTokens)
	assert.Equal(t, int64(cacheRead), usage.CacheReadInputTokens)
	assert.Equal(t, int64(cacheWrite), usage.CacheWriteInputTokens)
	assert.Equal(t, int64(outputReasoning), usage.ReasoningTokens)
}

func TestStreamRecorder_FilePartsPreserveObservedOrder(t *testing.T) {
	r := newRecorderForStreamTest()
	r.Observe(provider.StreamPart{
		Type:      provider.PartFile,
		Data:      &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte{1, 2, 3}},
		MediaType: "image/png",
		Filename:  "stream.png",
	})
	assert.False(t, r.FirstChunkAt().IsZero(), "file events are payload-bearing")
	r.Observe(provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: "before"})
	r.Observe(provider.StreamPart{Type: provider.PartReasoningStart, ID: "r0"})
	r.Observe(provider.StreamPart{Type: provider.PartReasoningDelta, ID: "r0", Delta: "after"})
	r.Observe(provider.StreamPart{
		Type:     provider.PartToolInputStart,
		ID:       "tc-1",
		ToolName: "lookup",
	})
	r.Observe(provider.StreamPart{Type: provider.PartToolInputDelta, ID: "tc-1", Delta: `{"q":"x"}`})
	r.Observe(provider.StreamPart{
		Type:       provider.PartToolCall,
		ToolCallID: "tc-1",
		ToolName:   "lookup",
		Input:      `{"q":"x"}`,
	})
	r.Observe(provider.StreamPart{
		Type:      provider.PartReasoningFile,
		Data:      &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte{1, 2, 3}},
		MediaType: "video/mp4",
		Filename:  "trace.mp4",
	})

	gen := r.Generation()
	require.Len(t, gen.Output, 1)
	require.Len(t, gen.Output[0].Parts, 5)
	assert.Equal(t, agento11y.PartKindMedia, gen.Output[0].Parts[0].Kind)
	assert.Equal(t, agento11y.PartKindText, gen.Output[0].Parts[1].Kind)
	assert.Equal(t, agento11y.PartKindThinking, gen.Output[0].Parts[2].Kind)
	assert.Equal(t, agento11y.PartKindToolCall, gen.Output[0].Parts[3].Kind)
	assert.Equal(t, agento11y.PartKindMedia, gen.Output[0].Parts[4].Kind)
	assert.Equal(t, "file", gen.Output[0].Parts[0].Metadata.ProviderType)
	assert.Equal(t, "reasoning_file", gen.Output[0].Parts[4].Metadata.ProviderType)
}

func TestStreamRecorder_TextReasoningSignature(t *testing.T) {
	r := newRecorderForStreamTest()
	r.Observe(provider.StreamPart{Type: provider.PartReasoningStart, ID: "r0"})
	r.Observe(provider.StreamPart{Type: provider.PartReasoningDelta, ID: "r0", Delta: "let me "})
	// Signature on later deltas — should be merged into the consolidated reasoning part.
	r.Observe(provider.StreamPart{
		Type:  provider.PartReasoningDelta,
		ID:    "r0",
		Delta: "think",
		ProviderMetadata: provider.ProviderMetadata{
			"anthropic": json.RawMessage(`{"signature":"sig-abc"}`),
		},
	})
	r.Observe(provider.StreamPart{
		Type: provider.PartReasoningDelta,
		ID:   "r0",
		// Same signature again — recorder dedupes to one copy.
		ProviderMetadata: provider.ProviderMetadata{
			"anthropic": json.RawMessage(`{"signature":"sig-abc"}`),
		},
	})
	r.Observe(provider.StreamPart{Type: provider.PartReasoningEnd, ID: "r0"})
	r.Observe(provider.StreamPart{Type: provider.PartTextStart, ID: "t0"})
	r.Observe(provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: "answer"})

	gen := r.Generation()
	require.Len(t, gen.Output, 1)
	require.Len(t, gen.Output[0].Parts, 2)

	thinking := gen.Output[0].Parts[0]
	assert.Equal(t, agento11y.PartKindThinking, thinking.Kind)
	assert.Equal(t, "let me think", thinking.Thinking)

	text := gen.Output[0].Parts[1]
	assert.Equal(t, agento11y.PartKindText, text.Kind)
	assert.Equal(t, "answer", text.Text)
}

func TestStreamRecorder_ToolCallDeltas(t *testing.T) {
	r := newRecorderForStreamTest()
	r.Observe(provider.StreamPart{
		Type:     provider.PartToolInputStart,
		ID:       "tc-1",
		ToolName: "lookup",
	})
	r.Observe(provider.StreamPart{Type: provider.PartToolInputDelta, ID: "tc-1", Delta: `{"q":`})
	r.Observe(provider.StreamPart{Type: provider.PartToolInputDelta, ID: "tc-1", Delta: `"sf"}`})
	r.Observe(provider.StreamPart{Type: provider.PartToolInputEnd, ID: "tc-1"})

	gen := r.Generation()
	require.Len(t, gen.Output, 1)
	require.Len(t, gen.Output[0].Parts, 1)
	call := gen.Output[0].Parts[0]
	assert.Equal(t, agento11y.PartKindToolCall, call.Kind)
	require.NotNil(t, call.ToolCall)
	assert.Equal(t, "tc-1", call.ToolCall.ID)
	assert.Equal(t, "lookup", call.ToolCall.Name)
	assert.JSONEq(t, `{"q":"sf"}`, string(call.ToolCall.InputJSON))
}

func TestStreamRecorder_ProviderExecutedToolCall(t *testing.T) {
	r := newRecorderForStreamTest()
	r.Observe(provider.StreamPart{
		Type:             provider.PartToolInputStart,
		ID:               "tc-1",
		ToolName:         "web_search",
		ProviderExecuted: true,
	})
	r.Observe(provider.StreamPart{
		Type:             provider.PartToolCall,
		ToolCallID:       "tc-1",
		ToolName:         "web_search",
		Input:            `{"query":"sf"}`,
		ProviderExecuted: true,
	})

	gen := r.Generation()
	require.Len(t, gen.Output, 1)
	require.Len(t, gen.Output[0].Parts, 1)
	call := gen.Output[0].Parts[0]
	assert.Equal(t, agento11y.PartKindToolCall, call.Kind)
	assert.Equal(t, "server_tool_use", call.Metadata.ProviderType)
}

func TestStreamRecorder_ToolResult(t *testing.T) {
	r := newRecorderForStreamTest()
	r.Observe(provider.StreamPart{
		Type:       provider.PartToolResult,
		ToolCallID: "tc-1",
		ToolName:   "lookup",
		Result:     json.RawMessage(`"sunny"`),
	})

	gen := r.Generation()
	require.Len(t, gen.Output, 1)
	msg := gen.Output[0]
	assert.Equal(t, agento11y.RoleTool, msg.Role)
	assert.Equal(t, "lookup", msg.Name)
	require.Len(t, msg.Parts, 1)
	result := msg.Parts[0]
	assert.Equal(t, agento11y.PartKindToolResult, result.Kind)
	assert.Equal(t, "tool_result", result.Metadata.ProviderType)
	require.NotNil(t, result.ToolResult)
	assert.Equal(t, "tc-1", result.ToolResult.ToolCallID)
	assert.JSONEq(t, `"sunny"`, string(result.ToolResult.ContentJSON))
}

func TestStreamRecorder_ToolCallFinalEvent(t *testing.T) {
	// Some providers send PartToolCall with the consolidated Input instead of
	// per-delta accumulation.
	r := newRecorderForStreamTest()
	r.Observe(provider.StreamPart{
		Type:       provider.PartToolCall,
		ToolCallID: "tc-1",
		ToolName:   "lookup",
		Input:      `{"q":"sf"}`,
	})
	gen := r.Generation()
	require.Len(t, gen.Output, 1)
	require.Len(t, gen.Output[0].Parts, 1)
	assert.JSONEq(t, `{"q":"sf"}`, string(gen.Output[0].Parts[0].ToolCall.InputJSON))
}

func TestStreamRecorder_FirstChunkAt(t *testing.T) {
	r := newRecorderForStreamTest()
	assert.True(t, r.FirstChunkAt().IsZero(), "no observations yet")

	r.Observe(provider.StreamPart{Type: provider.PartStreamStart})
	assert.True(t, r.FirstChunkAt().IsZero(), "bookkeeping events do not start the clock")

	r.Observe(provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: "x"})
	assert.False(t, r.FirstChunkAt().IsZero(), "payload event starts the clock")
}

func TestStreamRecorder_ErrorMidStream(t *testing.T) {
	r := newRecorderForStreamTest()
	r.Observe(provider.StreamPart{Type: provider.PartTextStart, ID: "t0"})
	r.Observe(provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: "hello"})
	apiErr := &provider.APICallError{Message: "upstream 500"}
	r.Observe(provider.StreamPart{Type: provider.PartError, APICallError: apiErr})

	require.Error(t, r.CallError())
	assert.True(t, errors.Is(r.CallError(), apiErr) || r.CallError().Error() != "")

	// Generation still includes the partial output observed before the error.
	gen := r.Generation()
	require.Len(t, gen.Output, 1)
	assert.Equal(t, "hello", gen.Output[0].Parts[0].Text)
}

func TestStreamRecorder_EmptyStream(t *testing.T) {
	r := newRecorderForStreamTest()
	gen := r.Generation()
	assert.Nil(t, gen.Output, "no observations -> nil output")
	assert.Empty(t, gen.StopReason)
	assert.Equal(t, agento11y.TokenUsage{}, gen.Usage)
}

func TestStreamRecorder_MultiStepFinish(t *testing.T) {
	// A second PartFinish in the stream wins (latest aggregate state).
	r := newRecorderForStreamTest()
	fr1 := provider.FinishReason{Unified: provider.FinishReasonToolCalls}
	r.Observe(provider.StreamPart{Type: provider.PartFinish, FinishReason: &fr1})
	gen1 := r.Generation()
	assert.Equal(t, "tool_use", gen1.StopReason)

	fr2 := provider.FinishReason{Unified: provider.FinishReasonStop}
	r.Observe(provider.StreamPart{Type: provider.PartFinish, FinishReason: &fr2})
	gen2 := r.Generation()
	assert.Equal(t, "end_turn", gen2.StopReason, "later finish wins")
}

func TestStreamRecorder_ResponseMetadataModelIdentity(t *testing.T) {
	tests := []struct {
		name                  string
		seedProvider          string
		seedModel             string
		responseProvider      string
		responseModel         string
		wantProvider          string
		wantModel             string
		wantTransportMetadata bool
	}{
		{
			name:                  "response metadata overrides transport identity",
			seedProvider:          "grafana",
			seedModel:             "claude-sonnet-4-5-20250929",
			responseProvider:      "anthropic",
			responseModel:         "claude-sonnet-4-5-20250929",
			wantProvider:          "anthropic",
			wantModel:             "claude-sonnet-4-5-20250929",
			wantTransportMetadata: true,
		},
		{
			name:          "incomplete response metadata keeps seed identity",
			seedProvider:  "grafana",
			seedModel:     "claude-sonnet-4-5-20250929",
			responseModel: "claude-sonnet-4-5-20250929",
			wantProvider:  "grafana",
			wantModel:     "claude-sonnet-4-5-20250929",
		},
		{
			name:             "direct provider matching identity omits transport metadata",
			seedProvider:     "anthropic",
			seedModel:        "claude-sonnet-4-5-20250929",
			responseProvider: "anthropic",
			responseModel:    "claude-sonnet-4-5-20250929",
			wantProvider:     "anthropic",
			wantModel:        "claude-sonnet-4-5-20250929",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewStreamRecorder(agento11y.GenerationStart{
				Model: agento11y.ModelRef{Provider: tc.seedProvider, Name: tc.seedModel},
			}, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})

			r.Observe(provider.StreamPart{
				Type:       provider.PartResponseMeta,
				Provider:   tc.responseProvider,
				ModelID:    tc.responseModel,
				ResponseID: "resp-1",
			})
			gen := r.Generation()

			assert.Equal(t, tc.wantProvider, gen.Model.Provider)
			assert.Equal(t, tc.wantModel, gen.Model.Name)
			assert.Equal(t, "resp-1", gen.ResponseID)
			if tc.wantTransportMetadata {
				require.NotNil(t, gen.Metadata)
				assert.Equal(t, tc.seedProvider, gen.Metadata[transportProviderMetadataKey])
				assert.Equal(t, tc.seedModel, gen.Metadata[transportModelMetadataKey])
			} else if gen.Metadata != nil {
				assert.NotContains(t, gen.Metadata, transportProviderMetadataKey)
				assert.NotContains(t, gen.Metadata, transportModelMetadataKey)
			}
		})
	}
}

func TestStreamRecorder_NilSafe(t *testing.T) {
	var r *StreamRecorder
	r.Observe(provider.StreamPart{Type: provider.PartTextDelta})
	assert.True(t, r.FirstChunkAt().IsZero())
	assert.Nil(t, r.CallError())
	assert.Equal(t, agento11y.Generation{}, r.Generation())
}

func TestAnthropicSignatureFromMetadata(t *testing.T) {
	tests := []struct {
		name string
		meta provider.ProviderMetadata
		want string
	}{
		{"nil", nil, ""},
		{"no anthropic key", provider.ProviderMetadata{"other": json.RawMessage(`{}`)}, ""},
		{
			"signature present",
			provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"sig-1"}`)},
			"sig-1",
		},
		{
			"no signature in object",
			provider.ProviderMetadata{"anthropic": json.RawMessage(`{"other":"x"}`)},
			"",
		},
		{
			"malformed JSON returns empty",
			provider.ProviderMetadata{"anthropic": json.RawMessage(`{garbage`)},
			"",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, anthropicSignatureFromMetadata(tc.meta))
		})
	}
}
