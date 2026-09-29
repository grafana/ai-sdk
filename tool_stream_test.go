package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolStream_PreliminaryFinalAndContinuation(t *testing.T) {
	var calls []provider.CallOptions
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		calls = append(calls, opts)
		if len(calls) == 1 {
			return &provider.StreamResult{Stream: toolCallStreamParts("lookup", `{}`)}, nil
		}
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}
	var converted []string
	tool := Tool{ExecuteStream: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions, emit func(json.RawMessage) error) error {
		if err := emit(json.RawMessage(`{"stage":1}`)); err != nil {
			return err
		}
		return emit(json.RawMessage(`{"stage":2}`))
	}, ToModelOutput: func(opts ToolOutputContext) (*provider.ToolResultOutput, error) {
		converted = append(converted, string(opts.Output))
		return &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: string(opts.Output)}, nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")),
		WithTools(ToolSet{"lookup": tool}), WithStopWhen(StepCountIs(2)))
	var results []StreamToolResult
	for part := range result.FullStream() {
		if tr, ok := part.(StreamToolResult); ok {
			results = append(results, tr)
		}
	}
	require.NoError(t, result.Err())
	require.Len(t, results, 3)
	assert.True(t, results[0].Preliminary)
	assert.True(t, results[1].Preliminary)
	assert.False(t, results[2].Preliminary)
	assert.JSONEq(t, `{"stage":2}`, string(results[2].Output))
	assert.Equal(t, []string{`{"stage":2}`}, converted)
	require.Len(t, result.Steps(), 2)
	require.Len(t, result.Steps()[0].ToolResults, 1)
	require.Len(t, calls, 2)
	assert.Equal(t, provider.RoleTool, calls[1].Prompt[len(calls[1].Prompt)-1].Role)
}

func TestToolStream_DynamicFinalMetadata(t *testing.T) {
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 2)
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "lookup", Input: `{}`, Title: "Inventory"}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")),
		WithTools(ToolSet{"lookup": {Type: UserToolDynamic, ExecuteStream: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions, emit func(json.RawMessage) error) error {
			return emit(json.RawMessage(`"done"`))
		}}}))
	var results []StreamToolResult
	for part := range result.FullStream() {
		if tr, ok := part.(StreamToolResult); ok {
			results = append(results, tr)
		}
	}
	require.NoError(t, result.Err())
	require.Len(t, results, 2)
	for _, tr := range results {
		require.NotNil(t, tr.Dynamic)
		assert.True(t, *tr.Dynamic)
		assert.Equal(t, "Inventory", tr.Title)
	}
}

func TestToolStream_PreliminaryDoesNotContinue(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		if calls.Add(1) > 1 {
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		}
		return &provider.StreamResult{Stream: toolCallStreamParts("lookup", `{}`)}, nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")),
		WithTools(ToolSet{"lookup": {ExecuteStream: func(ctx context.Context, _ json.RawMessage, _ ToolExecutionOptions, emit func(json.RawMessage) error) error {
			if err := emit(json.RawMessage(`"pending"`)); err != nil {
				return err
			}
			select {
			case <-release:
				return emit(json.RawMessage(`"done"`))
			case <-ctx.Done():
				return ctx.Err()
			}
		}}}), WithStopWhen(StepCountIs(2)))
	var prelimSeen bool
	for part := range result.FullStream() {
		if tr, ok := part.(StreamToolResult); ok && tr.Preliminary && !prelimSeen {
			prelimSeen = true
			assert.EqualValues(t, 1, calls.Load())
			close(release)
		}
	}
	require.NoError(t, result.Err())
	assert.True(t, prelimSeen)
}

func TestToolStream_ConcurrentEmission(t *testing.T) {
	release := make(chan struct{})
	var inCallback, concurrentCallback atomic.Bool
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 3)
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "slow", ToolName: "slow", Input: `{}`}
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "fast", ToolName: "fast", Input: `{}`}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")), WithTools(ToolSet{
		"slow": {ExecuteStream: func(ctx context.Context, _ json.RawMessage, _ ToolExecutionOptions, emit func(json.RawMessage) error) error {
			if err := emit(json.RawMessage(`"progress"`)); err != nil {
				return err
			}
			select {
			case <-release:
				return emit(json.RawMessage(`"finished"`))
			case <-ctx.Done():
				return ctx.Err()
			}
		}},
		"fast": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`"fast"`), nil
		}},
	}), OnChunk(func(state OnChunkState) {
		if _, ok := state.Chunk.(StreamToolResult); !ok {
			return
		}
		if !inCallback.CompareAndSwap(false, true) {
			concurrentCallback.Store(true)
			return
		}
		defer inCallback.Store(false)
		time.Sleep(time.Millisecond)
	}))
	var seenFast, seenSlowPrelim bool
	for part := range result.FullStream() {
		if tr, ok := part.(StreamToolResult); ok {
			if tr.ToolCallID == "fast" {
				seenFast = true
			}
			if tr.ToolCallID == "slow" && tr.Preliminary {
				seenSlowPrelim = true
			}
			if seenFast && seenSlowPrelim {
				select {
				case <-release:
				default:
					close(release)
				}
			}
		}
	}
	require.NoError(t, result.Err())
	assert.True(t, seenFast)
	assert.True(t, seenSlowPrelim)
	assert.False(t, concurrentCallback.Load())
	require.Len(t, result.ToolResults(), 2)
	assert.Equal(t, []string{"slow", "fast"}, []string{result.ToolResults()[0].ToolName, result.ToolResults()[1].ToolName})
}

func TestToolStream_CancellationAfterPreliminary(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: toolCallStreamParts("lookup", `{}`)}, nil
	}}
	result := StreamText(ctx, model, WithModelMessages(provider.UserText("hello")), WithTools(ToolSet{
		"lookup": {ExecuteStream: func(ctx context.Context, _ json.RawMessage, _ ToolExecutionOptions, emit func(json.RawMessage) error) error {
			if err := emit(json.RawMessage(`"progress"`)); err != nil {
				return err
			}
			<-ctx.Done()
			return ctx.Err()
		}},
	}))
	var final bool
	for part := range result.FullStream() {
		if tr, ok := part.(StreamToolResult); ok {
			if tr.Preliminary {
				cancel()
			} else {
				final = true
			}
		}
	}
	assert.False(t, final)
}

func TestToolStream_Approval(t *testing.T) {
	for _, tc := range []struct {
		name       string
		decision   ToolApprovalStatus
		wantRuns   int
		wantPrelim int
	}{
		{name: "approved", decision: ToolApprovalApproved, wantRuns: 1, wantPrelim: 1},
		{name: "pending", decision: ToolApprovalUserApproval},
		{name: "denied", decision: ToolApprovalDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runs := 0
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: toolCallStreamParts("lookup", `{}`)}, nil
			}}
			result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")),
				WithTools(ToolSet{"lookup": {ExecuteStream: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions, emit func(json.RawMessage) error) error {
					runs++
					return emit(json.RawMessage(`"done"`))
				}}}), WithToolApproval(ToolApprovalMap{"lookup": ApprovalPolicy(tc.decision)}))
			prelim := 0
			for part := range result.FullStream() {
				if tr, ok := part.(StreamToolResult); ok && tr.Preliminary {
					prelim++
				}
			}
			require.NoError(t, result.Err())
			assert.Equal(t, tc.wantRuns, runs)
			assert.Equal(t, tc.wantPrelim, prelim)
		})
	}
}

func TestToolStream_ApprovedResume(t *testing.T) {
	approved := true
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		last := opts.Prompt[len(opts.Prompt)-1]
		require.Equal(t, provider.RoleTool, last.Role)
		require.Len(t, last.Content, 1)
		assert.Equal(t, "finished", last.Content[0].Output.Text)
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(
		provider.NewAssistantMessage(provider.ToolCallPart("c1", "lookup", json.RawMessage(`{}`)),
			provider.ContentPart{Type: provider.ContentPartTypeToolApprovalRequest, ApprovalID: "approval-1", ToolCallID: "c1", ToolName: "lookup"}),
		provider.NewToolMessage(provider.ToolApprovalResponsePart("approval-1", approved, "")),
	), WithTools(ToolSet{"lookup": {ExecuteStream: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions, emit func(json.RawMessage) error) error {
		if err := emit(json.RawMessage(`"progress"`)); err != nil {
			return err
		}
		return emit(json.RawMessage(`"finished"`))
	}}}))
	prelim := 0
	for part := range result.FullStream() {
		if tr, ok := part.(StreamToolResult); ok && tr.Preliminary {
			prelim++
		}
	}
	require.NoError(t, result.Err())
	assert.Equal(t, 2, prelim)
}

func TestToolStream_EmptyErrorAndSingleResult(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tool       Tool
		wantPrelim int
		wantError  bool
		wantOutput string
	}{
		{name: "empty", tool: Tool{ExecuteStream: func(context.Context, json.RawMessage, ToolExecutionOptions, func(json.RawMessage) error) error {
			return nil
		}}},
		{name: "error", tool: Tool{ExecuteStream: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions, emit func(json.RawMessage) error) error {
			if err := emit(json.RawMessage(`"progress"`)); err != nil {
				return err
			}
			return errors.New("failed")
		}}, wantPrelim: 1, wantError: true},
		{name: "single", tool: Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`"done"`), nil
		}}, wantOutput: `"done"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: toolCallStreamParts("lookup", `{}`)}, nil
			}}
			result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")), WithTools(ToolSet{"lookup": tc.tool}))
			var prelim int
			for part := range result.FullStream() {
				if tr, ok := part.(StreamToolResult); ok && tr.Preliminary {
					prelim++
				}
			}
			require.NoError(t, result.Err())
			assert.Equal(t, tc.wantPrelim, prelim)
			require.Len(t, result.ToolResults(), 1)
			assert.Equal(t, tc.wantError, result.ToolResults()[0].IsError)
			assert.Equal(t, tc.wantOutput, string(result.ToolResults()[0].Output))
		})
	}
}
