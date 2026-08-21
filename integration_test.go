package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2EStreamToSSE(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("Hello from AI!")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
	)

	rec := httptest.NewRecorder()
	err := PipeUIMessageStreamToResponse(rec, result.ToUIMessageStream())
	require.NoError(t, err)

	body := rec.Body.String()
	lines := strings.Split(body, "\n\n")
	var dataLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, line)
		}
	}

	require.GreaterOrEqual(t, len(dataLines), 5, "expected at least 5 SSE events")

	var firstEvent map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(dataLines[0], "data: ")), &firstEvent))
	assert.Equal(t, "start", firstEvent["type"])

	foundDelta := false
	for _, line := range dataLines {
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(payload), &event))
		if event["type"] == "text-delta" && event["delta"] == "Hello from AI!" {
			foundDelta = true
		}
	}
	assert.True(t, foundDelta, "expected text-delta with 'Hello from AI!'")
	assert.True(t, strings.HasSuffix(body, "data: [DONE]\n\n"))
}

func TestE2EMultiStepToolLoop(t *testing.T) {
	callNum := 0
	model := &mockModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			callNum++
			if callNum == 1 {
				ch := make(chan provider.StreamPart, 10)
				go func() {
					defer close(ch)
					ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "weather", Input: `{"city":"London"}`}
					ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(15)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)}}}
				}()
				return &provider.StreamResult{Stream: ch}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("It's 20C in London")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("weather in London?")),
		WithTools(ToolSet{
			"weather": Tool{
				Description: "Get weather",
				InputSchema: testMustSchema(t, `{"type":"object","properties":{"city":{"type":"string"}}}`),
				Execute: func(_ context.Context, input json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{"temperature":"20C","city":"London"}`), nil
				},
			},
		}),
		WithStopWhen(StepCountIs(5)),
	)

	var events []string
	for part := range result.FullStream() {
		events = append(events, typeName(part))
	}

	var hasToolCall, hasToolResult bool
	var startSteps, finishSteps int
	for _, e := range events {
		switch e {
		case "tool-call":
			hasToolCall = true
		case "tool-result":
			hasToolResult = true
		case "start-step":
			startSteps++
		case "finish-step":
			finishSteps++
		}
	}

	assert.True(t, hasToolCall, "expected tool-call event")
	assert.True(t, hasToolResult, "expected tool-result event")
	assert.Equal(t, 2, startSteps)
	assert.Equal(t, 2, finishSteps)
	assert.Equal(t, "It's 20C in London", result.Text())
}

func TestE2EProviderExecutedToolFlow(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 10)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{
					Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "search",
					Input: `{"q":"test"}`, ProviderExecuted: true,
				}
				ch <- provider.StreamPart{
					Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "search",
					Result:           json.RawMessage(`"found"`),
					ProviderExecuted: true,
					ProviderMetadata: provider.ProviderMetadata{
						"grafana-ai-sdk": json.RawMessage(`{"customer":"keep"}`),
					},
				}
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
				ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "found results"}
				ch <- provider.StreamPart{
					Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
					Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(20)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(10)}},
				}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	var conversionContext ToolOutputContext
	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("search")),
		WithTools(ToolSet{
			"search": Tool{
				Description: "Search",
				ToModelOutput: func(ctx ToolOutputContext) (*provider.ToolResultOutput, error) {
					conversionContext = ctx
					return &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "converted"}, nil
				},
			},
		}),
		WithStopWhen(StepCountIs(5)),
	)

	var hasToolCall, hasToolResult bool
	for chunk := range result.ToUIMessageStream() {
		if chunk.Type == ChunkToolInputAvailable && chunk.ProviderExecuted {
			hasToolCall = true
		}
		if chunk.Type == ChunkToolOutputAvailable && chunk.ProviderExecuted {
			hasToolResult = true
		}
	}

	assert.True(t, hasToolCall, "expected provider-executed tool-input-available chunk")
	assert.True(t, hasToolResult, "expected provider-executed tool-output-available chunk")
	assert.Equal(t, "found results", result.Text())

	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].ToolResults, 1)
	toolResult := steps[0].ToolResults[0]
	assert.True(t, toolResult.ProviderExecuted)
	assert.JSONEq(t, `"found"`, string(toolResult.Output))
	assert.JSONEq(t, `{"customer":"keep"}`, string(toolResult.ProviderMetadata["grafana-ai-sdk"]))

	require.Len(t, steps[0].Response.Messages, 1)
	var promptResultPart provider.ContentPart
	var hasPromptResult bool
	for _, part := range steps[0].Response.Messages[0].Content {
		if part.Type == provider.ContentPartTypeToolResult {
			promptResultPart = part
			hasPromptResult = true
		}
	}
	require.True(t, hasPromptResult)
	require.NotNil(t, promptResultPart.Output)
	assert.Nil(t, promptResultPart.ProviderExecuted)
	promptResultJSON, err := json.Marshal(promptResultPart)
	require.NoError(t, err)
	assert.NotContains(t, string(promptResultJSON), "providerExecuted")
	assert.Equal(t, provider.ToolOutputText, promptResultPart.Output.Type)
	assert.Equal(t, "converted", promptResultPart.Output.Text)
	assert.Equal(t, "c1", conversionContext.ToolCallID)
	assert.JSONEq(t, `{"q":"test"}`, string(conversionContext.Input))
	assert.JSONEq(t, `"found"`, string(conversionContext.Output))
}

func TestE2EProviderExecutedToolErrorFlow(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 3)
			ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "search", Input: `{}`, ProviderExecuted: true}
			ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "search", Result: json.RawMessage(`"failed"`), IsError: true}
			ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("search")),
		WithTools(ToolSet{"search": {Description: "Search"}}),
	)

	var hasAvailable, hasError bool
	for chunk := range result.ToUIMessageStream(OnUIMessageStreamError(func(error) string { return "redacted" })) {
		hasAvailable = hasAvailable || chunk.Type == ChunkToolOutputAvailable
		if chunk.Type == ChunkToolOutputError {
			hasError = true
			assert.Equal(t, "failed", chunk.ErrorText)
		}
	}
	assert.False(t, hasAvailable)
	assert.True(t, hasError)

	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].ToolResults, 1)
	assert.True(t, steps[0].ToolResults[0].IsError)
	assert.JSONEq(t, `"failed"`, string(steps[0].ToolResults[0].Output))
	require.Len(t, steps[0].Response.Messages, 1)
	assert.Equal(t, provider.ToolOutputErrorJSON, steps[0].Response.Messages[0].Content[1].Output.Type)
	assert.JSONEq(t, `"failed"`, string(steps[0].Response.Messages[0].Content[1].Output.JSON))
}

func TestE2EProviderExecutedToolResultConversionErrorOrdering(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 2)
			ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "search", Input: `{"q":"test"}`, ProviderExecuted: true}
			ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "search", Result: json.RawMessage(`"found"`)}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	var callbackOrder []string
	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("search")),
		WithTools(ToolSet{
			"search": {
				Description: "Search",
				ToModelOutput: func(ToolOutputContext) (*provider.ToolResultOutput, error) {
					callbackOrder = append(callbackOrder, "convert")
					return nil, errors.New("conversion failed")
				},
			},
		}),
		OnChunk(func(state OnChunkState) {
			if _, ok := state.Chunk.(StreamToolResult); ok {
				callbackOrder = append(callbackOrder, "chunk")
			}
		}),
	)

	var streamOrder []string
	for part := range result.FullStream() {
		switch part.(type) {
		case StreamToolResult:
			streamOrder = append(streamOrder, "result")
		case StreamError:
			streamOrder = append(streamOrder, "error")
		}
	}
	assert.Equal(t, []string{"chunk", "convert"}, callbackOrder)
	assert.Equal(t, []string{"result", "error"}, streamOrder)
	assert.ErrorContains(t, result.Err(), "converting provider-executed tool result: conversion failed")
}

func TestE2EProviderExecutedPreliminaryToolResults(t *testing.T) {
	preliminary := true
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 4)
			ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "image", Input: `{}`, ProviderExecuted: true}
			ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "image", Result: json.RawMessage(`"preview"`), Preliminary: &preliminary}
			ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "image", Result: json.RawMessage(`"final"`)}
			ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("create image")),
		WithTools(ToolSet{"image": {Description: "Image"}}),
	)

	var streamed []StreamToolResult
	for part := range result.FullStream() {
		if toolResult, ok := part.(StreamToolResult); ok {
			streamed = append(streamed, toolResult)
		}
	}
	require.Len(t, streamed, 2)
	assert.False(t, streamed[0].Preliminary)
	assert.False(t, streamed[1].Preliminary)

	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].ToolResults, 1)
	assert.JSONEq(t, `"final"`, string(steps[0].ToolResults[0].Output))
	require.Len(t, steps[0].Response.Messages, 1)
	assert.Equal(t, "final", steps[0].Response.Messages[0].Content[1].Output.Text)
}

func TestE2EProviderDefinedToolInToolSet(t *testing.T) {
	t.Run("tool arrives at DoStream with correct type and ID", func(t *testing.T) {
		var capturedTools []provider.Tool
		model := &mockModel{
			streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				capturedTools = opts.Tools
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("search")),
			WithTools(ToolSet{
				"search": Tool{
					Type: UserToolProvider,
					ID:   "anthropic.web_search_20250305",
					Args: map[string]json.RawMessage{
						"maxUses": json.RawMessage(`5`),
					},
				},
				"weather": Tool{
					Description: "Get weather",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
				},
			}),
		)
		for range result.ToUIMessageStream() {
		}

		require.Len(t, capturedTools, 2)
		assert.Equal(t, provider.ToolTypeProvider, capturedTools[0].Type)
		assert.Equal(t, "search", capturedTools[0].Name)
		assert.Equal(t, "anthropic.web_search_20250305", capturedTools[0].ID)
		assert.JSONEq(t, `5`, string(capturedTools[0].Args["maxUses"]))
		assert.Equal(t, provider.ToolTypeFunction, capturedTools[1].Type)
		assert.Equal(t, "weather", capturedTools[1].Name)
	})

	t.Run("callbacks fire for provider tools", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, 10)
				go func() {
					defer close(ch)
					ch <- provider.StreamPart{
						Type: provider.PartToolInputStart, ID: "c1", ToolName: "search",
						ProviderExecuted: true,
					}
					ch <- provider.StreamPart{
						Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "search",
						Input: `{"q":"test"}`, ProviderExecuted: true,
					}
					ch <- provider.StreamPart{
						Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "search",
						Result:           json.RawMessage(`{"results":["a"]}`),
						ProviderExecuted: true,
					}
					ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "search done"}
					ch <- provider.StreamPart{
						Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
						Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)}},
					}
				}()
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		var inputStartCalled, inputAvailableCalled bool
		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("search")),
			WithTools(ToolSet{
				"search": Tool{
					Type: UserToolProvider,
					ID:   "anthropic.web_search_20250305",
					OnInputStart: func(_ ToolExecutionOptions) {
						inputStartCalled = true
					},
					OnInputAvailable: func(_ json.RawMessage, _ ToolExecutionOptions) {
						inputAvailableCalled = true
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)
		for range result.ToUIMessageStream() {
		}

		assert.True(t, inputStartCalled, "OnInputStart should fire for provider tool")
		assert.True(t, inputAvailableCalled, "OnInputAvailable should fire for provider tool")
		assert.Equal(t, "search done", result.Text())

		steps := result.Steps()
		require.Len(t, steps, 1)
		require.Len(t, steps[0].ToolCalls, 1)
		assert.True(t, steps[0].ToolCalls[0].ProviderExecuted)
	})
}

func TestE2EHTTPRoundTrip(t *testing.T) {
	inputMsgs := []UIMessage{
		{ID: "m1", Role: RoleUser, Parts: []Part{TextPart{Text: "hello"}}},
	}

	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("goodbye")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithMessages(inputMsgs...),
	)

	rec := httptest.NewRecorder()
	stream := result.ToUIMessageStream()
	require.NoError(t, PipeUIMessageStreamToResponse(rec, stream))

	body := rec.Body.String()
	assert.Contains(t, body, `"text-delta"`)
	assert.Contains(t, body, `"goodbye"`)
}
