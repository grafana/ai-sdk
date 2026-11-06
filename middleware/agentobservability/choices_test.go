package agentobservability

import (
	"context"
	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChoices_IsolatedOutputsAndGlobalUsage(t *testing.T) {
	zero, one := 0, 1
	stop := provider.FinishReason{Unified: provider.FinishReasonStop}
	usage := provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(30)}}
	recorder := NewStreamRecorder(agento11y.GenerationStart{}, provider.CallOptions{ChoiceCount: 2})
	for _, part := range []provider.StreamPart{
		{Type: provider.PartTextDelta, ChoiceIndex: &one, ID: "same", Delta: "one"},
		{Type: provider.PartTextDelta, ChoiceIndex: &zero, ID: "same", Delta: "zero"},
		{Type: provider.PartFinish, ChoiceIndex: &zero, FinishReason: &stop, Usage: &provider.Usage{}},
		{Type: provider.PartTextDelta, ChoiceIndex: &one, ID: "same", Delta: "!"},
		{Type: provider.PartFinish, ChoiceIndex: &one, FinishReason: &stop, Usage: &provider.Usage{}},
		{Type: provider.PartChoicesFinish, Usage: &usage},
	} {
		recorder.Observe(part)
	}
	got := recorder.Generation()
	require.Len(t, got.Output, 2)
	assert.Equal(t, "zero", got.Output[0].Parts[0].Text)
	assert.Equal(t, "one!", got.Output[1].Parts[0].Text)
	assert.Equal(t, usageToAgento11y(usage), got.Usage)
	assert.Equal(t, stopReasonEndTurn, got.StopReason)
	expected := MapGenerateResult(provider.CallOptions{ChoiceCount: 2}, &provider.GenerateResult{Usage: usage, Choices: []provider.GenerateChoice{{Index: 1, Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "one!"}}, FinishReason: stop}, {Index: 0, Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "zero"}}, FinishReason: stop}}}, ContextInfo{})
	assert.Equal(t, expected.Output, got.Output)
	assert.Equal(t, expected.Metadata["ai_sdk.choices"], got.Metadata["ai_sdk.choices"])
	assert.Equal(t, stop, recorder.currentState().finishReasonValue())
}

func TestChoices_ToolIDsDoNotMerge(t *testing.T) {
	zero, one := 0, 1
	r := NewStreamRecorder(agento11y.GenerationStart{}, provider.CallOptions{ChoiceCount: 2})
	for _, part := range []provider.StreamPart{
		{Type: provider.PartToolInputStart, ChoiceIndex: &zero, ID: "same", ToolName: "first"},
		{Type: provider.PartToolInputStart, ChoiceIndex: &one, ID: "same", ToolName: "second"},
		{Type: provider.PartToolInputDelta, ChoiceIndex: &zero, ID: "same", Delta: `{"x":0}`},
		{Type: provider.PartToolInputDelta, ChoiceIndex: &one, ID: "same", Delta: `{"x":1}`},
		{Type: provider.PartToolCall, ChoiceIndex: &zero, ToolCallID: "same", ToolName: "first", Input: `{"x":0}`},
		{Type: provider.PartToolCall, ChoiceIndex: &one, ToolCallID: "same", ToolName: "second", Input: `{"x":1}`},
	} {
		r.Observe(part)
	}
	out := r.Generation().Output
	require.Len(t, out, 2)
	require.NotNil(t, out[0].Parts[0].ToolCall)
	require.NotNil(t, out[1].Parts[0].ToolCall)
	assert.Equal(t, "same", out[0].Parts[0].ToolCall.ID)
	assert.Equal(t, "same", out[1].Parts[0].ToolCall.ID)
	assert.JSONEq(t, `{"x":0}`, string(out[0].Parts[0].ToolCall.InputJSON))
	assert.JSONEq(t, `{"x":1}`, string(out[1].Parts[0].ToolCall.InputJSON))
}

func TestChoices_CancellationBeforeGlobalFinish(t *testing.T) {
	env := testkit.NewEnv(t)
	zero := 0
	finish := provider.FinishReason{Unified: provider.FinishReasonStop}
	upstream := make(chan provider.StreamPart, 1)
	upstream <- provider.StreamPart{Type: provider.PartFinish, ChoiceIndex: &zero, FinishReason: &finish, Usage: &provider.Usage{}}
	t.Cleanup(func() { close(upstream) })
	stream := startRecordedStream(t, env, upstream, 0)
	readUntil(t, stream.result.Stream, provider.PartFinish)
	stream.cancel()
	stream.waitRecorded(t)
	require.NoError(t, env.Client.Shutdown(context.Background()))
	gen := env.SingleGenerationJSON(t)
	assert.Contains(t, gen["call_error"], "context canceled")
}
