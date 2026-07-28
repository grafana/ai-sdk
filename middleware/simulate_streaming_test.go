package middleware

import (
	"context"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimulateStreaming(t *testing.T) {
	t.Run("TextContent_ProducesCorrectStreamParts", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentText, Text: "hello world"},
					},
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
					Usage: provider.Usage{
						InputTokens:  provider.InputTokenUsage{Total: ptr(10)},
						OutputTokens: provider.OutputTokenUsage{Total: ptr(5)},
					},
				}, nil
			},
		}

		wrapped := WrapLanguageModel(model, SimulateStreaming())
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var parts []provider.StreamPart
		for p := range result.Stream {
			parts = append(parts, p)
		}

		require.Len(t, parts, 6)
		assert.Equal(t, provider.PartStreamStart, parts[0].Type)
		assert.Equal(t, provider.PartResponseMeta, parts[1].Type)
		assert.Equal(t, provider.PartTextStart, parts[2].Type)
		assert.Equal(t, "0", parts[2].ID)
		assert.Equal(t, provider.PartTextDelta, parts[3].Type)
		assert.Equal(t, "hello world", parts[3].Delta)
		assert.Equal(t, "0", parts[3].ID)
		assert.Equal(t, provider.PartTextEnd, parts[4].Type)
		assert.Equal(t, "0", parts[4].ID)
		assert.Equal(t, provider.PartFinish, parts[5].Type)
		assert.Equal(t, provider.FinishReasonStop, parts[5].FinishReason.Unified)
		assert.Equal(t, 5, *parts[5].Usage.OutputTokens.Total)
	})

	t.Run("ReasoningContent_ProducesReasoningEvents", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentReasoning, Text: "thinking..."},
						{Type: provider.ContentText, Text: "answer"},
					},
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
				}, nil
			},
		}

		wrapped := WrapLanguageModel(model, SimulateStreaming())
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var parts []provider.StreamPart
		for p := range result.Stream {
			parts = append(parts, p)
		}

		require.Len(t, parts, 9)
		assert.Equal(t, provider.PartStreamStart, parts[0].Type)
		assert.Equal(t, provider.PartResponseMeta, parts[1].Type)
		assert.Equal(t, provider.PartReasoningStart, parts[2].Type)
		assert.Equal(t, "0", parts[2].ID)
		assert.Equal(t, provider.PartReasoningDelta, parts[3].Type)
		assert.Equal(t, "thinking...", parts[3].Delta)
		assert.Equal(t, provider.PartReasoningEnd, parts[4].Type)
		assert.Equal(t, provider.PartTextStart, parts[5].Type)
		assert.Equal(t, "1", parts[5].ID)
		assert.Equal(t, provider.PartTextDelta, parts[6].Type)
		assert.Equal(t, "answer", parts[6].Delta)
		assert.Equal(t, provider.PartTextEnd, parts[7].Type)
		assert.Equal(t, provider.PartFinish, parts[8].Type)
	})

	t.Run("NonTextContent_PassedThrough", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{
							Type:             provider.ContentToolCall,
							ToolCallID:       "tc1",
							ToolName:         "search",
							Input:            []byte(`{"q":"test"}`),
							ProviderExecuted: true,
							Dynamic:          ptr(true),
							Kind:             "function",
						},
					},
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls},
				}, nil
			},
		}

		wrapped := WrapLanguageModel(model, SimulateStreaming())
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var parts []provider.StreamPart
		for p := range result.Stream {
			parts = append(parts, p)
		}

		require.Len(t, parts, 4)
		assert.Equal(t, provider.PartStreamStart, parts[0].Type)
		assert.Equal(t, provider.PartResponseMeta, parts[1].Type)
		tc := parts[2]
		assert.Equal(t, provider.StreamPartType("tool-call"), tc.Type)
		assert.Equal(t, "tc1", tc.ToolCallID)
		assert.Equal(t, "search", tc.ToolName)
		assert.Equal(t, `{"q":"test"}`, tc.Input)
		assert.True(t, tc.ProviderExecuted, "ProviderExecuted preserved")
		require.NotNil(t, tc.Dynamic, "Dynamic preserved")
		assert.True(t, *tc.Dynamic, "Dynamic preserved")
		assert.Equal(t, "function", tc.Kind, "Kind preserved")
		assert.Equal(t, provider.PartFinish, parts[3].Type)
	})

	t.Run("SourceContent_FieldsPreserved", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{
							Type:       provider.ContentSource,
							SourceType: provider.SourceTypeURL,
							URL:        "https://example.com",
						},
					},
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
				}, nil
			},
		}

		wrapped := WrapLanguageModel(model, SimulateStreaming())
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var parts []provider.StreamPart
		for p := range result.Stream {
			parts = append(parts, p)
		}

		require.Len(t, parts, 4)
		src := parts[2]
		assert.Equal(t, provider.StreamPartType("source"), src.Type)
		require.NotNil(t, src.Source)
		assert.Equal(t, provider.SourceTypeURL, src.Source.SourceType)
		assert.Equal(t, "https://example.com", src.Source.URL)
	})

	t.Run("DoGenerate_PassesThroughUnmodified", func(t *testing.T) {
		generateResult := &provider.GenerateResult{
			Content: []provider.GenerateContentPart{
				{Type: provider.ContentText, Text: "direct"},
			},
		}
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return generateResult, nil
			},
		}

		wrapped := WrapLanguageModel(model, SimulateStreaming())
		got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		assert.Equal(t, generateResult, got)
	})

	t.Run("ResponseMetadata_Preserved", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hi"}},
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
					Request:      &provider.RequestMetadata{},
					Response: &provider.GenerateResponse{
						ResponseMetadata: provider.ResponseMetadata{
							ID:      "resp-123",
							ModelID: "test-model",
						},
						Headers: map[string]string{"x-req-id": "abc"},
					},
				}, nil
			},
		}

		wrapped := WrapLanguageModel(model, SimulateStreaming())
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		require.NotNil(t, result.Request)
		require.NotNil(t, result.Response)
		assert.Equal(t, "abc", result.Response.Headers["x-req-id"])

		var parts []provider.StreamPart
		for p := range result.Stream {
			parts = append(parts, p)
		}
		require.True(t, len(parts) >= 2)
		responseMeta := parts[1]
		assert.Equal(t, provider.PartResponseMeta, responseMeta.Type)
		assert.Equal(t, "resp-123", responseMeta.ResponseID)
		assert.Equal(t, "test-model", responseMeta.ModelID)
	})

	t.Run("ResponseMetadata_EmittedWhenNilResponse", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hi"}},
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
				}, nil
			},
		}

		wrapped := WrapLanguageModel(model, SimulateStreaming())
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var parts []provider.StreamPart
		for p := range result.Stream {
			parts = append(parts, p)
		}

		require.True(t, len(parts) >= 2)
		assert.Equal(t, provider.PartResponseMeta, parts[1].Type, "response-metadata always emitted even when Response is nil")
	})

	t.Run("EmptyText_Skipped", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentText, Text: ""},
						{Type: provider.ContentText, Text: "actual"},
					},
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
				}, nil
			},
		}

		wrapped := WrapLanguageModel(model, SimulateStreaming())
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var parts []provider.StreamPart
		for p := range result.Stream {
			parts = append(parts, p)
		}

		require.Len(t, parts, 6)
		assert.Equal(t, provider.PartStreamStart, parts[0].Type)
		assert.Equal(t, provider.PartResponseMeta, parts[1].Type)
		assert.Equal(t, provider.PartTextStart, parts[2].Type)
		assert.Equal(t, "0", parts[2].ID)
		assert.Equal(t, provider.PartTextDelta, parts[3].Type)
		assert.Equal(t, "actual", parts[3].Delta)
		assert.Equal(t, provider.PartTextEnd, parts[4].Type)
		assert.Equal(t, provider.PartFinish, parts[5].Type)
	})

	t.Run("GeneratedFileDataVariants", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentFile, Data: &provider.DataContent{Base64: "AQID"}, MediaType: "image/png"},
						{Type: provider.ContentReasoningFile, Data: &provider.DataContent{URL: "https://example.com/reasoning.png"}, MediaType: "image/png"},
						{Type: provider.ContentFile, Data: &provider.DataContent{Bytes: []byte{}}, MediaType: "application/octet-stream"},
					},
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
				}, nil
			},
		}

		wrapped := WrapLanguageModel(model, SimulateStreaming())
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var files []provider.StreamPart
		for part := range result.Stream {
			if part.Type == provider.PartFile || part.Type == provider.PartReasoningFile {
				files = append(files, part)
			}
		}

		require.Len(t, files, 3)
		require.NotNil(t, files[0].Data)
		assert.Equal(t, provider.StreamFileData{Type: provider.StreamFileDataTypeData, Base64: "AQID"}, *files[0].Data)
		require.NotNil(t, files[1].Data)
		assert.Equal(t, provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "https://example.com/reasoning.png"}, *files[1].Data)
		require.NotNil(t, files[2].Data)
		assert.Equal(t, provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte{}}, *files[2].Data)
	})

	t.Run("ProviderMetadata_OnFinish", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content:          []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hi"}},
					FinishReason:     provider.FinishReason{Unified: provider.FinishReasonStop},
					ProviderMetadata: provider.ProviderMetadata{"custom": []byte(`{"key":"value"}`)},
				}, nil
			},
		}

		wrapped := WrapLanguageModel(model, SimulateStreaming())
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var finish provider.StreamPart
		for p := range result.Stream {
			if p.Type == provider.PartFinish {
				finish = p
			}
		}
		assert.Equal(t, provider.PartFinish, finish.Type)
		assert.NotNil(t, finish.ProviderMetadata)
		assert.JSONEq(t, `{"key":"value"}`, string(finish.ProviderMetadata["custom"]))
	})

	t.Run("Warnings_InStreamStart", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hi"}},
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
					Warnings: []provider.Warning{
						{Type: provider.WarnUnsupported, Feature: "logprobs"},
					},
				}, nil
			},
		}

		wrapped := WrapLanguageModel(model, SimulateStreaming())
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		first := <-result.Stream
		assert.Equal(t, provider.PartStreamStart, first.Type)
		require.Len(t, first.Warnings, 1)
		assert.Equal(t, "logprobs", first.Warnings[0].Feature)

		for range result.Stream {
		}
	})
}
