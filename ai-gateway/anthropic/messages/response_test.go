package messages

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ParallelToolStreams(t *testing.T) {
	for _, interleaved := range []bool{false, true} {
		name := "deferred ends"
		if interleaved {
			name = "interleaved deltas"
		}
		t.Run(name, func(t *testing.T) {
			parts := []provider.StreamPart{
				{Type: provider.PartToolInputStart, ID: "a", ToolName: "weather"},
				{Type: provider.PartToolInputDelta, ID: "a", Delta: `{"city":`},
			}
			tail := provider.StreamPart{Type: provider.PartToolInputDelta, ID: "a", Delta: `"Oslo"}`}
			if !interleaved {
				parts = append(parts, tail)
			}
			parts = append(parts,
				provider.StreamPart{Type: provider.PartToolInputStart, ID: "b", ToolName: "weather"},
				provider.StreamPart{Type: provider.PartToolInputDelta, ID: "b", Delta: `{"city":"Rome"}`},
			)
			if interleaved {
				parts = append(parts, tail)
			}
			parts = append(parts,
				provider.StreamPart{Type: provider.PartToolInputEnd, ID: "a"},
				provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "a", ToolName: "weather", Input: `{"city":"Oslo"}`},
				provider.StreamPart{Type: provider.PartToolInputEnd, ID: "b"},
				provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "b", ToolName: "weather", Input: `{"city":"Rome"}`},
				finish(provider.FinishReasonToolCalls),
			)
			server := httptest.NewServer(testHandler(t, &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return streamOf(parts...), nil
			}}))
			defer server.Close()
			client := anthropicsdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
			stream := client.Messages.NewStreaming(t.Context(), anthropicsdk.MessageNewParams{
				Model: "alias", MaxTokens: 16,
				Messages: []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("weather?"))},
			})
			t.Cleanup(func() { assert.NoError(t, stream.Close()) })
			var acc anthropicsdk.Message
			var stopped []int64
			var complete bool
			for stream.Next() {
				event := stream.Current()
				require.NoError(t, acc.Accumulate(event))
				if event.Type == "content_block_stop" {
					stopped = append(stopped, event.Index)
				}
				complete = complete || event.Type == "message_stop"
			}
			require.NoError(t, stream.Err())
			assert.True(t, complete)
			assert.Equal(t, []int64{0, 1}, stopped)
			assert.Equal(t, anthropicsdk.StopReasonToolUse, acc.StopReason)
			require.Len(t, acc.Content, 2)
			assert.Equal(t, "a", acc.Content[0].ID)
			assert.JSONEq(t, `{"city":"Oslo"}`, string(acc.Content[0].Input))
			assert.Equal(t, "b", acc.Content[1].ID)
			assert.JSONEq(t, `{"city":"Rome"}`, string(acc.Content[1].Input))
		})
	}
}

func TestStopReason_Mapping(t *testing.T) {
	for _, tc := range []struct {
		unified provider.UnifiedFinishReason
		raw     string
		want    stopReasonType
		ok      bool
	}{
		{provider.FinishReasonStop, "end_turn", stopReasonEndTurn, true},
		{provider.FinishReasonStop, "pause_turn", stopReasonPauseTurn, true},
		{provider.FinishReasonStop, "stop_sequence", stopReasonStopSequence, true},
		{provider.FinishReasonLength, "max_tokens", stopReasonMaxTokens, true},
		{provider.FinishReasonToolCalls, "tool_calls", stopReasonToolUse, true},
		{provider.FinishReasonContentFilter, "refusal", stopReasonRefusal, true},
		{provider.FinishReasonContentFilter, "content_filter", stopReasonRefusal, true},
		{provider.FinishReasonOther, "compaction", "", false},
		{provider.FinishReasonError, "error", "", false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, ok := stopReason(provider.FinishReason{Unified: tc.unified, Raw: tc.raw})
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.ok, ok)
		})
	}
}

func TestHandler_Refusal(t *testing.T) {
	server := httptest.NewServer(testHandler(t, &fakeModel{
		generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			result := textResult()
			result.FinishReason = provider.FinishReason{Unified: provider.FinishReasonContentFilter, Raw: "refusal"}
			return result, nil
		},
		stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return streamOf(
				provider.StreamPart{Type: provider.PartTextStart, ID: "t"},
				provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: "hello"},
				provider.StreamPart{Type: provider.PartTextEnd, ID: "t"},
				finish(provider.FinishReasonContentFilter),
			), nil
		},
	}))
	defer server.Close()
	client := anthropicsdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
	params := anthropicsdk.MessageNewParams{Model: "alias", MaxTokens: 16, Messages: []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("hi"))}}
	msg, err := client.Messages.New(t.Context(), params)
	require.NoError(t, err)
	assert.Equal(t, anthropicsdk.StopReasonRefusal, msg.StopReason)
	stream := client.Messages.NewStreaming(t.Context(), params)
	t.Cleanup(func() { assert.NoError(t, stream.Close()) })
	var acc anthropicsdk.Message
	var complete bool
	for stream.Next() {
		require.NoError(t, acc.Accumulate(stream.Current()))
		complete = complete || stream.Current().Type == "message_stop"
	}
	require.NoError(t, stream.Err())
	assert.True(t, complete)
	assert.Equal(t, anthropicsdk.StopReasonRefusal, acc.StopReason)
}

func TestHandler_StreamSetupDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	h := testHandler(t, &fakeModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	req := httptest.NewRequest("POST", Path, strings.NewReader(`{"model":"alias","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, 504, w.Code)
	assert.JSONEq(t, `{"type":"error","error":{"type":"timeout_error","message":"request timed out"}}`, w.Body.String())
}

func TestMapRequest_TextBlockBoundaries(t *testing.T) {
	_, options, err := mapRequest([]byte(`{"model":"alias","max_tokens":8,
		"system":[{"type":"text","text":"first"},{"type":"text","text":"second"}],
		"messages":[{"role":"user","content":"hi"},
		{"role":"system","content":[{"type":"text","text":"third"},{"type":"text","text":"fourth"}]},
		{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"read","input":{}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","is_error":true,"content":[{"type":"text","text":"line one"},{"type":"text","text":"line two"}]}]}]}`))
	require.NoError(t, err)
	require.Len(t, options.Prompt, 7)
	for i, want := range map[int]string{0: "first", 1: "second", 3: "third", 4: "fourth"} {
		assert.Equal(t, provider.NewSystemMessage(want), options.Prompt[i])
	}
	assert.Equal(t, provider.NewToolMessage(provider.ToolResultPart("a", "read", &provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "line one\nline two"})), options.Prompt[6])
}
