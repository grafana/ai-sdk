package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/provider"
)

var projectionCitation = provider.ProviderMetadata{"anthropic": json.RawMessage(`{"citations":[{"url":"https://a.test"}]}`)}

func projectionToolStepParts() <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 16)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartReasoningStart, ID: "r1"}
		ch <- provider.StreamPart{Type: provider.PartReasoningDelta, ID: "r1", Delta: "plan"}
		ch <- provider.StreamPart{Type: provider.PartReasoningEnd, ID: "r1"}
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "checking"}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1", ProviderMetadata: projectionCitation}
		ch <- provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "s1", URL: "https://a.test"}}
		ch <- provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte("chart")}, MediaType: "text/plain"}
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "weather", Input: `{"city":"NYC"}`}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(3)}}}
	}()
	return ch
}

func projectionFinalStepParts() <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 16)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartReasoningStart, ID: "r2"}
		ch <- provider.StreamPart{Type: provider.PartReasoningDelta, ID: "r2", Delta: "answer"}
		ch <- provider.StreamPart{Type: provider.PartReasoningEnd, ID: "r2"}
		ch <- provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "s2", URL: "https://b.test"}}
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t2"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t2", Delta: "sunny"}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t2"}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)}}}
	}()
	return ch
}

func projectionModel() *mockModel {
	model := &mockModel{}
	model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		if model.callCount == 1 {
			return &provider.StreamResult{Stream: projectionToolStepParts()}, nil
		}
		return &provider.StreamResult{Stream: projectionFinalStepParts()}, nil
	}
	return model
}

func projectionTools(t *testing.T) ToolSet {
	t.Helper()
	return ToolSet{"weather": {
		InputSchema: testMustSchema(t, `{"type":"object"}`),
		Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`{"temp":72}`), nil
		},
	}}
}

type projectedResult struct {
	text             string
	reasoning        []ReasoningOutput
	content          []ContentPart
	toolCalls        []ToolCall
	toolResults      []ToolResult
	files            []GeneratedFile
	sources          []Source
	steps            []StepResult
	response         ResponseMetadata
	responseMessages []provider.Message
}

func projectStream(result *StreamTextResult) projectedResult {
	return projectedResult{
		text:             result.Text(),
		reasoning:        result.Reasoning(),
		content:          result.Content(),
		toolCalls:        result.ToolCalls(),
		toolResults:      result.ToolResults(),
		files:            result.Files(),
		sources:          result.Sources(),
		steps:            result.Steps(),
		response:         result.Response(),
		responseMessages: result.ResponseMessages(),
	}
}

func projectGenerate(result *GenerateTextResult) projectedResult {
	return projectedResult{
		text:             result.Text,
		reasoning:        result.Reasoning,
		content:          result.Content,
		toolCalls:        result.ToolCalls,
		toolResults:      result.ToolResults,
		files:            result.Files,
		sources:          result.Sources,
		steps:            result.Steps,
		response:         result.Response,
		responseMessages: result.ResponseMessages,
	}
}

func TestStreamTextResult_AllStepProjection(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T) projectedResult
	}{
		{
			name: "stream text",
			run: func(t *testing.T) projectedResult {
				result := StreamText(t.Context(), projectionModel(), WithModelMessages(provider.UserText("weather?")), WithTools(projectionTools(t)), WithStopWhen(StepCountIs(3)))
				for range result.FullStream() {
				}
				require.NoError(t, result.Err())
				return projectStream(result)
			},
		},
		{
			name: "generate text",
			run: func(t *testing.T) projectedResult {
				result, err := GenerateText(t.Context(), projectionModel(), WithModelMessages(provider.UserText("weather?")), WithTools(projectionTools(t)), WithStopWhen(StepCountIs(3)))
				require.NoError(t, err)
				return projectGenerate(result)
			},
		},
		{
			name: "agent stream",
			run: func(t *testing.T) projectedResult {
				agent := NewToolLoopAgent(projectionModel(), WithToolLoopAgentOptions(WithTools(projectionTools(t))))
				result := agent.Stream(t.Context(), WithAgentPrompt("weather?"))
				for range result.FullStream() {
				}
				require.NoError(t, result.Err())
				return projectStream(result)
			},
		},
		{
			name: "agent generate",
			run: func(t *testing.T) projectedResult {
				agent := NewToolLoopAgent(projectionModel(), WithToolLoopAgentOptions(WithTools(projectionTools(t))))
				result, err := agent.Generate(t.Context(), WithAgentPrompt("weather?"))
				require.NoError(t, err)
				return projectGenerate(result)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.run(t)
			require.Len(t, got.steps, 2)
			first, final := got.steps[0], got.steps[1]

			assert.Equal(t, "sunny", got.text)
			require.Len(t, got.reasoning, 1)
			assert.Equal(t, ReasoningTextOutput{Text: "answer"}, got.reasoning[0])
			assert.Equal(t, final.Response, got.response)

			assert.Equal(t, append(append([]ContentPart{}, first.Content...), final.Content...), got.content)
			assert.Contains(t, got.content, ContentPart(TextContent{Text: "checking", ProviderMetadata: projectionCitation}))

			require.Len(t, got.toolCalls, 1)
			assert.Equal(t, "c1", got.toolCalls[0].ToolCallID)
			require.Len(t, got.toolResults, 1)
			assert.Equal(t, "c1", got.toolResults[0].ToolCallID)
			require.Len(t, got.files, 1)
			assert.Equal(t, []byte("chart"), got.files[0].Data)
			require.Len(t, got.sources, 2)
			assert.Equal(t, "s1", got.sources[0].ID)
			assert.Equal(t, "s2", got.sources[1].ID)

			assert.Equal(t, append(cloneMessages(first.Response.Messages), final.Response.Messages...), got.responseMessages)
			require.Len(t, got.responseMessages, 3)
			assert.Equal(t, provider.RoleAssistant, got.responseMessages[0].Role)
			assert.Equal(t, provider.RoleTool, got.responseMessages[1].Role)
			assert.Equal(t, provider.RoleAssistant, got.responseMessages[2].Role)
		})
	}
}

func TestStreamTextResult_ResponseMessagesIncludeApprovalResults(t *testing.T) {
	msgs := []provider.Message{
		provider.UserText("transfer"),
		provider.NewAssistantMessage(
			provider.ToolCallPart("c1", "transfer", json.RawMessage(`{"amount":1}`)),
			provider.ToolApprovalRequestPart("apr_1", "c1", false),
			provider.ToolCallPart("c2", "transfer", json.RawMessage(`{"amount":2}`)),
			provider.ToolApprovalRequestPart("apr_2", "c2", false),
		),
		provider.NewToolMessage(
			provider.ToolApprovalResponsePart("apr_1", true, "ok"),
			provider.ToolApprovalResponsePart("apr_2", false, "too much"),
		),
	}
	tools := ToolSet{"transfer": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
		return json.RawMessage(`{"ok":true}`), nil
	}}}
	newModel := func() *mockModel {
		return &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		}}
	}

	check := func(t *testing.T, got projectedResult) {
		t.Helper()
		require.Len(t, got.steps, 1)
		require.Len(t, got.responseMessages, 2)
		initial := got.responseMessages[0]
		assert.Equal(t, provider.RoleTool, initial.Role)
		require.Len(t, initial.Content, 2)
		assert.Equal(t, provider.ContentPartTypeToolResult, initial.Content[0].Type)
		assert.Equal(t, "c1", initial.Content[0].ToolCallID)
		assert.Equal(t, provider.ContentPartTypeToolResult, initial.Content[1].Type)
		assert.Equal(t, "c2", initial.Content[1].ToolCallID)
		require.NotNil(t, initial.Content[1].Output)
		assert.Equal(t, provider.ToolOutputExecutionDenied, initial.Content[1].Output.Type)
		assert.Equal(t, got.steps[0].Response.Messages, got.responseMessages[1:])
		assert.Equal(t, "done", got.text)
		assert.Empty(t, got.toolResults)
	}

	t.Run("stream text", func(t *testing.T) {
		result := StreamText(t.Context(), newModel(), WithModelMessages(msgs...), WithTools(tools))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())
		check(t, projectStream(result))
	})
	t.Run("generate text", func(t *testing.T) {
		result, err := GenerateText(t.Context(), newModel(), WithModelMessages(msgs...), WithTools(tools))
		require.NoError(t, err)
		check(t, projectGenerate(result))
	})
}

func TestStreamTextResult_ProjectionEdgeCases(t *testing.T) {
	t.Run("no completed step", func(t *testing.T) {
		model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return nil, errors.New("boom")
		}}
		result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hi")), WithMaxRetries(0))
		for range result.FullStream() {
		}
		require.Error(t, result.Err())
		assert.Empty(t, result.Steps())
		assert.Nil(t, result.Content())
		assert.Nil(t, result.ToolCalls())
		assert.Nil(t, result.ToolResults())
		assert.Nil(t, result.Files())
		assert.Nil(t, result.Sources())
		assert.Empty(t, result.ResponseMessages())
	})

	t.Run("error after completed step retains earlier steps", func(t *testing.T) {
		model := &mockModel{}
		model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			if model.callCount == 1 {
				return &provider.StreamResult{Stream: projectionToolStepParts()}, nil
			}
			return nil, errors.New("boom")
		}
		result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hi")), WithTools(projectionTools(t)), WithStopWhen(StepCountIs(3)), WithMaxRetries(0))
		for range result.FullStream() {
		}
		require.Error(t, result.Err())
		steps := result.Steps()
		require.Len(t, steps, 1)
		assert.Equal(t, steps[0].Content, result.Content())
		assert.Equal(t, steps[0].ToolCalls, result.ToolCalls())
		assert.Equal(t, steps[0].Response.Messages, result.ResponseMessages())
	})

	t.Run("cancellation after completed step retains earlier steps", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		model := &mockModel{}
		model.streamFunc = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			if model.callCount == 1 {
				return &provider.StreamResult{Stream: projectionToolStepParts()}, nil
			}
			cancel()
			return nil, ctx.Err()
		}
		result := StreamText(ctx, model, WithModelMessages(provider.UserText("hi")), WithTools(projectionTools(t)), WithStopWhen(StepCountIs(3)), WithMaxRetries(0))
		for range result.FullStream() {
		}
		steps := result.Steps()
		require.Len(t, steps, 1)
		assert.Equal(t, steps[0].Sources, result.Sources())
		assert.Equal(t, steps[0].ToolResults, result.ToolResults())
		assert.Equal(t, steps[0].Response.Messages, result.ResponseMessages())
	})

	t.Run("returned collections are caller owned", func(t *testing.T) {
		result := StreamText(t.Context(), projectionModel(), WithModelMessages(provider.UserText("hi")), WithTools(projectionTools(t)), WithStopWhen(StepCountIs(3)))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())

		content := result.Content()
		content[0] = TextContent{Text: "mutated"}
		calls := result.ToolCalls()
		calls[0].ToolCallID = "mutated"
		sources := result.Sources()
		sources[0].ID = "mutated"
		messages := result.ResponseMessages()
		messages[0] = provider.UserText("mutated")
		_ = append(messages[:1], provider.UserText("appended"))

		assert.NotEqual(t, content, result.Content())
		assert.Equal(t, "c1", result.ToolCalls()[0].ToolCallID)
		assert.Equal(t, "s1", result.Sources()[0].ID)
		assert.Equal(t, provider.RoleAssistant, result.ResponseMessages()[0].Role)
		assert.Equal(t, result.Steps()[0].Response.Messages, result.ResponseMessages()[:2])
		assert.Equal(t, "c1", result.Steps()[0].ToolCalls[0].ToolCallID)
	})
}
