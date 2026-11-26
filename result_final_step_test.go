package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/provider"
)

func TestFinalStep_MultiStep(t *testing.T) {
	input := provider.UserText("weather?")
	for _, entry := range []string{"stream", "generate", "agent generate"} {
		t.Run(entry, func(t *testing.T) {
			model := finalStepModel()
			tools := projectionTools(t)
			stop := WithStopWhen(StepCountIs(2))
			var got StepResult
			var ok bool
			var steps []StepResult
			switch entry {
			case "stream":
				result := StreamText(t.Context(), model, WithModelMessages(input), WithTools(tools), stop)
				for range result.FullStream() {
				}
				require.NoError(t, result.Err())
				got, ok = result.FinalStep()
				steps = result.Steps()
			case "generate":
				result, err := GenerateText(t.Context(), model, WithModelMessages(input), WithTools(tools), stop)
				require.NoError(t, err)
				got, ok = result.FinalStep()
				steps = result.Steps
			case "agent generate":
				agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(tools), stop))
				result, err := agent.Generate(t.Context(), WithAgentPrompt("weather?"))
				require.NoError(t, err)
				got, ok = result.FinalStep()
				steps = result.Steps
			}

			require.True(t, ok)
			require.Len(t, steps, 2)
			assert.Equal(t, steps[1], got)
			assert.Equal(t, "sunny", got.Text)
			assert.Equal(t, "answer", got.ReasoningText)
			require.Len(t, got.Reasoning, 1)
			assert.Equal(t, json.RawMessage(`{"step":2}`), got.Request.Body)
			assert.Equal(t, "final-response", got.Response.ID)
			assert.Equal(t, "second", got.Response.Headers["x-step"])
			require.Len(t, got.Response.Messages, 2)
			assert.Equal(t, provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: "tool_use"}, got.FinishReason)
			require.Len(t, got.ToolCalls, 1)
			require.Len(t, got.ToolResults, 1)
			assert.Equal(t, "final-tool", got.ToolCalls[0].ToolCallID)
			assert.Equal(t, "final-tool", got.ToolResults[0].ToolCallID)
			require.Len(t, got.Files, 1)
			assert.Equal(t, []byte("final file"), got.Files[0].Data)
			assert.Len(t, got.Content, 5)
			assert.Equal(t, []provider.Warning{{Type: provider.WarnOther, Message: "final warning"}}, got.Warnings)
			assert.Equal(t, intPtr(7), got.Usage.InputTokens.Total)
			assert.Equal(t, json.RawMessage(`{"final":true}`), got.ProviderMetadata["mock"])
		})
	}
}

func finalStepModel() *mockModel {
	model := &mockModel{}
	model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		if model.callCount == 1 {
			return &provider.StreamResult{Stream: projectionToolStepParts()}, nil
		}
		metadata := provider.ProviderMetadata{"mock": json.RawMessage(`{"final":true}`)}
		return &provider.StreamResult{
			Request: &provider.RequestMetadata{Body: json.RawMessage(`{"step":2}`)},
			Stream: projectionParts(
				provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnOther, Message: "final warning"}}},
				provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "final-response", Timestamp: time.Unix(2, 0), ResponseHeaders: map[string]string{"x-step": "second"}},
				provider.StreamPart{Type: provider.PartReasoningStart, ID: "r2"},
				provider.StreamPart{Type: provider.PartReasoningDelta, ID: "r2", Delta: "answer"},
				provider.StreamPart{Type: provider.PartReasoningEnd, ID: "r2", ProviderMetadata: metadata},
				provider.StreamPart{Type: provider.PartTextStart, ID: "t2"},
				provider.StreamPart{Type: provider.PartTextDelta, ID: "t2", Delta: "sunny"},
				provider.StreamPart{Type: provider.PartTextEnd, ID: "t2", ProviderMetadata: metadata, Citations: []provider.Citation{{Source: provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "citation", URL: "https://final.test", ProviderMetadata: metadata}, StartIndex: intPtr(0)}}},
				provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte("final file")}, MediaType: "text/plain"},
				provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "final-tool", ToolName: "weather", Input: `{"city":"LA"}`, ProviderMetadata: metadata},
				provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: "tool_use"}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(7)}, Raw: json.RawMessage(`{"tokens":7}`)}, ProviderMetadata: metadata},
			),
		}, nil
	}
	return model
}

func TestFinalStep_Completion(t *testing.T) {
	t.Run("zero results", func(t *testing.T) {
		for _, f := range []func() (StepResult, bool){(&StreamTextResult{}).FinalStep, (&GenerateTextResult{}).FinalStep} {
			step, ok := f()
			assert.False(t, ok)
			assert.Equal(t, StepResult{}, step)
		}
	})

	t.Run("waits without claiming the stream", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stream := make(chan provider.StreamPart)
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: stream}, nil
			}}
			result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")))
			returned := make(chan StepResult, 1)
			go func() {
				step, ok := result.FinalStep()
				assert.True(t, ok)
				returned <- step
			}()
			synctest.Wait()
			assert.Empty(t, returned)

			drained := make(chan bool, 1)
			go func() {
				sawFinish := false
				for part := range result.FullStream() {
					if _, ok := part.(StreamFinish); ok {
						sawFinish = true
					}
				}
				drained <- sawFinish
			}()
			for part := range textStreamParts("complete") {
				stream <- part
			}
			close(stream)
			assert.Equal(t, "complete", (<-returned).Text)
			assert.True(t, <-drained)
		})
	})

	boom := errors.New("provider failed")
	for _, tc := range []struct {
		name      string
		completed bool
		mode      string
		wantErr   error
		wantText  string
	}{
		{name: "initial error", mode: "error", wantErr: boom},
		{name: "continuation error", completed: true, mode: "error", wantErr: boom, wantText: "checking"},
		{name: "empty continuation EOF", completed: true, mode: "empty EOF", wantErr: ErrNoOutputGenerated, wantText: "checking"},
		{name: "partial continuation EOF", completed: true, mode: "partial EOF", wantText: "partial"},
		{name: "initial mid-step cancellation", mode: "cancel"},
		{name: "continuation mid-step cancellation", completed: true, mode: "cancel", wantText: "checking"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			model := &mockModel{}
			model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				if tc.completed && model.callCount == 1 {
					return &provider.StreamResult{Stream: projectionToolStepParts()}, nil
				}
				switch tc.mode {
				case "error":
					return nil, boom
				case "empty EOF":
					return &provider.StreamResult{Stream: projectionParts(provider.StreamPart{Type: provider.PartStreamStart})}, nil
				}
				pending := make(chan provider.StreamPart, 2)
				pending <- provider.StreamPart{Type: provider.PartTextStart, ID: "unfinished"}
				pending <- provider.StreamPart{Type: provider.PartTextDelta, ID: "unfinished", Delta: "partial"}
				if tc.mode == "partial EOF" {
					close(pending)
				}
				return &provider.StreamResult{Stream: pending}, nil
			}
			result := StreamText(ctx, model, WithModelMessages(provider.UserText("hi")), WithTools(projectionTools(t)),
				WithStopWhen(StepCountIs(3)), WithMaxRetries(0), OnChunk(func(state OnChunkState) {
					if delta, ok := state.Chunk.(StreamTextDelta); ok && delta.ID == "unfinished" && tc.mode == "cancel" {
						cancel()
					}
				}))
			var aborted bool
			for part := range result.FullStream() {
				if _, ok := part.(StreamAbort); ok {
					aborted = true
				}
			}
			assert.ErrorIs(t, result.Err(), tc.wantErr)
			assert.Equal(t, tc.mode == "cancel", aborted)
			final, ok := result.FinalStep()
			steps := result.Steps()
			if tc.mode == "partial EOF" {
				require.Len(t, steps, 2)
				assert.Equal(t, provider.FinishReasonOther, final.FinishReason.Unified)
			} else if tc.completed {
				require.Len(t, steps, 1)
			} else {
				assert.Empty(t, steps)
				assert.False(t, ok)
				assert.Equal(t, StepResult{}, final)
				return
			}
			require.True(t, ok)
			assert.Equal(t, tc.wantText, final.Text)
			assert.Equal(t, steps[len(steps)-1], final)
		})
	}

	for _, tc := range []struct {
		name   string
		cancel bool
		err    error
	}{
		{name: "error", err: boom},
		{name: "cancellation", cancel: true, err: context.Canceled},
	} {
		t.Run("collected "+tc.name+" returns an error without a result", func(t *testing.T) {
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, boom }}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			result, err := GenerateText(ctx, model, WithModelMessages(provider.UserText("hi")), WithMaxRetries(0))
			assert.ErrorIs(t, err, tc.err)
			assert.Nil(t, result)
			agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithMaxRetries(0)))
			result, err = agent.Generate(ctx, WithAgentPrompt("hi"))
			assert.ErrorIs(t, err, tc.err)
			assert.Nil(t, result)
		})
	}
}

func TestFinalStep_ValueCopyAndConcurrentReads(t *testing.T) {
	input := WithModelMessages(provider.UserText("weather?"))
	tools := WithTools(projectionTools(t))
	stop := WithStopWhen(StepCountIs(2))
	stream := StreamText(t.Context(), finalStepModel(), input, tools, stop)
	for range stream.FullStream() {
	}
	require.NoError(t, stream.Err())
	collected, err := GenerateText(t.Context(), finalStepModel(), input, tools, stop)
	require.NoError(t, err)

	for _, tc := range []struct {
		name  string
		final func() (StepResult, bool)
		steps []StepResult
	}{
		{name: "stream", final: stream.FinalStep, steps: stream.Steps()},
		{name: "generate", final: collected.FinalStep, steps: collected.Steps},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Len(t, tc.steps, 2)
			want := tc.steps[1]

			var readers sync.WaitGroup
			for range 4 {
				readers.Go(func() {
					step, ok := tc.final()
					assert.True(t, ok)
					assert.Equal(t, want, step)
				})
			}
			readers.Wait()

			step, ok := tc.final()
			require.True(t, ok)
			step.Text = "changed"
			step.Files = nil
			step.Response.Headers = nil
			assert.NotEqual(t, want, step)
			again, ok := tc.final()
			assert.True(t, ok)
			assert.Equal(t, want, tc.steps[1])
			assert.Equal(t, want, again)
		})
	}
}
