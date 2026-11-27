package v4

import (
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamingInterruptedFinish(t *testing.T) {
	interrupted := segmentFinish(provider.FinishReasonInterrupted, 4, 2, `{"input_tokens":4}`, `[{"partial":true}]`)
	stop := segmentFinish(provider.FinishReasonStop, 8, 3, "", "")
	for _, tc := range []struct {
		name   string
		parts  []provider.StreamPart
		frames []string
	}{
		{
			name:  "text",
			parts: segmentText("0", "partial")[:2],
			frames: []string{
				`data: {"type":"text-start","id":"0"}`,
				`data: {"type":"text-delta","id":"0","delta":"partial"}`,
			},
		},
		{
			name:  "reasoning",
			parts: segmentReasoning("0")[:2],
			frames: []string{
				`data: {"type":"reasoning-start","id":"0"}`,
				`data: {"type":"reasoning-delta","id":"0","delta":"thinking"}`,
			},
		},
		{
			name: "tool input",
			parts: []provider.StreamPart{
				{Type: provider.PartToolInputStart, ID: "0", ToolName: "lookup"},
				{Type: provider.PartToolInputDelta, ID: "0", Delta: "{"},
			},
			frames: []string{
				`data: {"type":"tool-input-start","id":"0","toolName":"lookup"}`,
				`data: {"type":"tool-input-delta","id":"0","delta":"{"}`,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const finish = `{"type":"finish","finishReason":{"unified":"interrupted","raw":"interrupted"},"usage":{"inputTokens":{"total":4},"outputTokens":{"total":2},"raw":{"input_tokens":4}},"providerMetadata":{"anthropic":{"safeguardResults":[{"partial":true}]}}}`
			prefix := string(canonicalEmptyStartFrame) + tc.frames[0] + "\n\n" + tc.frames[1] + "\n\n"
			body := serveSegmentParts(t, false, joinSegmentParts(tc.parts, []provider.StreamPart{interrupted})...)
			frame, ok := strings.CutPrefix(body, prefix+"data: ")
			require.True(t, ok, body)
			assert.JSONEq(t, finish, frame)
			body = serveSegmentParts(t, false, joinSegmentParts(tc.parts, []provider.StreamPart{stop})...)
			assert.Equal(t, prefix+string(canonicalInternalStreamErrorFrame), body)
		})
	}

	t.Run("next segment", func(t *testing.T) {
		body := serveSegmentParts(t, false, joinSegmentParts(
			segmentText("0", "partial")[:2],
			[]provider.StreamPart{interrupted, segmentMeta("msg_2")},
			segmentText("0", "complete"),
			[]provider.StreamPart{stop},
		)...)
		assert.Equal(t, []string{"stream-start", "text-start", "text-delta", "finish", "response-metadata", "text-start", "text-delta", "text-end", "finish"}, segmentFrameTypes(t, body))
	})

	t.Run("late end without metadata", func(t *testing.T) {
		body := serveSegmentParts(t, false, joinSegmentParts(
			segmentText("0", "partial")[:2],
			[]provider.StreamPart{interrupted, {Type: provider.PartTextEnd, ID: "0"}},
		)...)
		assert.Equal(t, []string{"stream-start", "text-start", "text-delta", "finish", "error"}, segmentFrameTypes(t, body))
		assert.Equal(t, []string{"internal_error"}, segmentErrorCodes(t, body))
	})

	t.Run("unfinished previews", func(t *testing.T) {
		preview := provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "lookup", Preliminary: new(true), Result: []byte(`{"status":"pending"}`)}
		for _, tc := range []struct {
			name   string
			prompt string
			parts  []provider.StreamPart
			types  []string
		}{
			{"stream", `{"prompt":[]}`, []provider.StreamPart{
				{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: "{}", ProviderExecuted: true},
				preview,
			}, []string{"stream-start", "tool-call", "tool-result", "error"}},
			{"history", `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerExecuted":true}]}]}`, []provider.StreamPart{preview}, []string{"stream-start", "tool-result", "error"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				body := serveSegmentRequest(t, streamRequest(tc.prompt), append(tc.parts, interrupted)...)
				require.Equal(t, tc.types, segmentFrameTypes(t, body))
				assert.Equal(t, []string{"internal_error"}, segmentErrorCodes(t, body))
			})
		}
	})
}
