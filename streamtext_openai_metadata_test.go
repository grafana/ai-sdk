package aisdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolCallMetadata_ThroughGenerateAndStreamContinuation(t *testing.T) {
	for _, mode := range []string{"generate", "stream"} {
		for _, async := range []string{"true", "false"} {
			t.Run(mode+"/async="+async, func(t *testing.T) {
				metadata := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"fc_1","async":` + async + `,"caller":{"type":"program","callerId":"prog_1"}}`)}
				var continuation []provider.Message
				calls := 0
				model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
					calls++
					if calls == 1 {
						ch := make(chan provider.StreamPart, 2)
						ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call_1", ToolName: "lookup", Input: `{}`, ProviderMetadata: metadata}
						ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
						close(ch)
						return &provider.StreamResult{Stream: ch}, nil
					}
					continuation = opts.Prompt
					return &provider.StreamResult{Stream: textStreamParts("done")}, nil
				}}
				tools := WithTools(ToolSet{"lookup": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{"ok":true}`), nil
				}}})
				if mode == "generate" {
					result, err := GenerateText(t.Context(), model,
						WithModelMessages(provider.UserText("look up")), tools, WithStopWhen(StepCountIs(2)))
					require.NoError(t, err)
					assert.Equal(t, "done", result.Text)
				} else {
					result := StreamText(t.Context(), model,
						WithModelMessages(provider.UserText("look up")), tools, WithStopWhen(StepCountIs(2)))
					var available *UIMessageChunk
					for chunk := range result.ToUIMessageStream() {
						if chunk.Type == ChunkToolInputAvailable {
							available = &chunk
						}
					}
					require.NoError(t, result.Err())
					require.NotNil(t, available)
					assert.JSONEq(t, string(metadata["openai"]), string(available.ProviderMetadata["openai"]))
				}
				require.Equal(t, 2, calls)
				require.Len(t, continuation, 3)
				require.Len(t, continuation[1].Content, 1)
				call := continuation[1].Content[0]
				assert.Equal(t, provider.ContentPartTypeToolCall, call.Type)
				callMetadata, ok := call.ProviderOptions["openai"].(provider.RawProviderOption)
				require.True(t, ok)
				assert.JSONEq(t, string(metadata["openai"]), string(callMetadata.Raw))
				require.Len(t, continuation[2].Content, 1)
				resultMetadata, ok := continuation[2].Content[0].ProviderOptions["openai"].(provider.RawProviderOption)
				require.True(t, ok)
				assert.JSONEq(t, string(metadata["openai"]), string(resultMetadata.Raw))
			})
		}
	}
}

func TestProgrammaticDenialMetadata_ThroughGenerateAndStreamContinuation(t *testing.T) {
	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			metadata := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"fc_1","async":true,"caller":{"type":"program","callerId":"prog_1"}}`)}
			calls := 0
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				if calls == 1 {
					ch := make(chan provider.StreamPart, 2)
					ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call_1", ToolName: "dangerous", Input: `{}`, ProviderMetadata: metadata}
					ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
					close(ch)
					return &provider.StreamResult{Stream: ch}, nil
				}
				require.Len(t, opts.Prompt, 3)
				callMetadata, ok := opts.Prompt[1].Content[0].ProviderOptions["openai"].(provider.RawProviderOption)
				require.True(t, ok)
				assert.JSONEq(t, string(metadata["openai"]), string(callMetadata.Raw))
				var denied bool
				for _, part := range opts.Prompt[2].Content {
					if part.Output != nil && part.Output.Type == provider.ToolOutputExecutionDenied && part.ToolCallID == "call_1" {
						denied = true
					}
				}
				assert.True(t, denied)
				return &provider.StreamResult{Stream: textStreamParts("denied")}, nil
			}}
			tools := WithTools(ToolSet{"dangerous": {}})
			approval := WithToolApproval(ToolApprovalMap{"dangerous": ApprovalPolicy(ToolApprovalDenied, "blocked")})
			if mode == "generate" {
				result, err := GenerateText(t.Context(), model,
					WithModelMessages(provider.UserText("do not run")), tools, approval, WithStopWhen(StepCountIs(2)))
				require.NoError(t, err)
				assert.Equal(t, "denied", result.Text)
			} else {
				result := StreamText(t.Context(), model,
					WithModelMessages(provider.UserText("do not run")), tools, approval, WithStopWhen(StepCountIs(2)))
				for range result.FullStream() {
				}
				require.NoError(t, result.Err())
			}
			assert.Equal(t, 2, calls)
		})
	}
}
