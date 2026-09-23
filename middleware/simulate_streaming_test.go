package middleware

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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
		timestamp := time.Unix(1710000000, 0).UTC()
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hi"}},
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
					Request:      &provider.RequestMetadata{},
					Response: &provider.GenerateResponse{
						ResponseMetadata: provider.ResponseMetadata{
							ID:        "resp-123",
							ModelID:   "test-model",
							Timestamp: timestamp,
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
		assert.Equal(t, timestamp, responseMeta.Timestamp)
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

func TestSimulateStreaming_ContentProjection(t *testing.T) {
	metadata := provider.ProviderMetadata{"test": json.RawMessage(`{"signature":"signed"}`)}
	finish := provider.FinishReason{Unified: provider.FinishReasonStop}
	usage := provider.Usage{OutputTokens: provider.OutputTokenUsage{Total: ptr(3)}}
	warnings := []provider.Warning{{Type: provider.WarnUnsupported, Feature: "logprobs"}}

	tests := []struct {
		name       string
		content    []provider.GenerateContentPart
		want       []provider.StreamPart
		sourceJSON string
	}{
		{
			name: "text with metadata",
			content: []provider.GenerateContentPart{
				{Type: provider.ContentText, Text: "hello", ProviderMetadata: metadata},
			},
			want: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "0", ProviderMetadata: metadata},
				{Type: provider.PartTextDelta, ID: "0", Delta: "hello"},
				{Type: provider.PartTextEnd, ID: "0"},
			},
		},
		{
			name: "empty text consumes no ID",
			content: []provider.GenerateContentPart{
				{Type: provider.ContentText, ProviderMetadata: metadata},
				{Type: provider.ContentText, Text: "next", ProviderMetadata: metadata},
			},
			want: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "0", ProviderMetadata: metadata},
				{Type: provider.PartTextDelta, ID: "0", Delta: "next"},
				{Type: provider.PartTextEnd, ID: "0"},
			},
		},
		{
			name:    "empty reasoning consumes ID",
			content: []provider.GenerateContentPart{{Type: provider.ContentReasoning, ProviderMetadata: metadata}},
			want: []provider.StreamPart{
				{Type: provider.PartReasoningStart, ID: "0", ProviderMetadata: metadata},
				{Type: provider.PartReasoningDelta, ID: "0"},
				{Type: provider.PartReasoningEnd, ID: "0"},
			},
		},
		{
			name: "tool call",
			content: []provider.GenerateContentPart{{
				Type: provider.ContentToolCall, ToolCallID: "call-1", ToolName: "lookup",
				Input: json.RawMessage(`{"q":"go"}`), ProviderExecuted: true,
				Dynamic: ptr(false), ProviderMetadata: metadata,
			}},
			want: []provider.StreamPart{{
				Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "lookup",
				Input: `{"q":"go"}`, ProviderExecuted: true,
				Dynamic: ptr(false), ProviderMetadata: metadata,
			}},
		},
		{
			name: "tool result error with preliminary true",
			content: []provider.GenerateContentPart{{
				Type: provider.ContentToolResult, ToolCallID: "call-1", ToolName: "lookup",
				Result: json.RawMessage(`{"error":"unavailable"}`), IsError: true,
				Preliminary: ptr(true), Dynamic: ptr(true), ProviderMetadata: metadata,
			}},
			want: []provider.StreamPart{{
				Type: provider.PartToolResult, ToolCallID: "call-1", ToolName: "lookup",
				Result: json.RawMessage(`{"error":"unavailable"}`), IsError: true,
				Preliminary: ptr(true), Dynamic: ptr(true), ProviderMetadata: metadata,
			}},
		},
		{
			name: "tool result with preliminary false",
			content: []provider.GenerateContentPart{{
				Type: provider.ContentToolResult, ToolCallID: "call-2", ToolName: "lookup",
				Result: json.RawMessage(`null`), Preliminary: ptr(false),
			}},
			want: []provider.StreamPart{{
				Type: provider.PartToolResult, ToolCallID: "call-2", ToolName: "lookup",
				Result: json.RawMessage(`null`), Preliminary: ptr(false),
			}},
		},
		{
			name: "approval request",
			content: []provider.GenerateContentPart{{
				Type: provider.ContentToolApprovalRequest, ApprovalID: "approval-1",
				ToolCallID: "call-1", ProviderMetadata: metadata,
			}},
			want: []provider.StreamPart{{
				Type: provider.PartToolApprovalRequest, ApprovalID: "approval-1",
				ToolCallID: "call-1", ProviderMetadata: metadata,
			}},
		},
		{
			name: "URL source",
			content: []provider.GenerateContentPart{{
				Type: provider.ContentSource, SourceType: provider.SourceTypeURL,
				ID: "source-url", URL: "https://example.com", Title: "Example", ProviderMetadata: metadata,
			}},
			want: []provider.StreamPart{{
				Type: provider.PartSource,
				Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "source-url",
					URL: "https://example.com", Title: "Example", ProviderMetadata: metadata},
			}},
			sourceJSON: `{"type":"source","sourceType":"url","id":"source-url","url":"https://example.com","title":"Example","providerMetadata":{"test":{"signature":"signed"}}}`,
		},
		{
			name: "document source",
			content: []provider.GenerateContentPart{{
				Type: provider.ContentSource, SourceType: provider.SourceTypeDocument,
				ID: "source-doc", Title: "Report", MediaType: "application/pdf",
				Filename: "report.pdf", ProviderMetadata: metadata,
			}},
			want: []provider.StreamPart{{
				Type: provider.PartSource,
				Source: &provider.SourceInfo{SourceType: provider.SourceTypeDocument,
					ID: "source-doc", Title: "Report", MediaType: "application/pdf",
					Filename: "report.pdf", ProviderMetadata: metadata},
			}},
			sourceJSON: `{"type":"source","sourceType":"document","id":"source-doc","title":"Report","mediaType":"application/pdf","filename":"report.pdf","providerMetadata":{"test":{"signature":"signed"}}}`,
		},
		{
			name:    "file bytes",
			content: []provider.GenerateContentPart{{Type: provider.ContentFile, Data: &provider.DataContent{Bytes: []byte{1, 2}}, MediaType: "image/png", ProviderMetadata: metadata}},
			want:    []provider.StreamPart{{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte{1, 2}}, MediaType: "image/png", ProviderMetadata: metadata}},
		},
		{
			name:    "file base64",
			content: []provider.GenerateContentPart{{Type: provider.ContentFile, Data: &provider.DataContent{Base64: "AQID"}, MediaType: "image/png"}},
			want:    []provider.StreamPart{{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Base64: "AQID"}, MediaType: "image/png"}},
		},
		{
			name:    "file empty bytes",
			content: []provider.GenerateContentPart{{Type: provider.ContentFile, Data: &provider.DataContent{Bytes: []byte{}}, MediaType: "image/png"}},
			want:    []provider.StreamPart{{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte{}}, MediaType: "image/png"}},
		},
		{
			name:    "file URL",
			content: []provider.GenerateContentPart{{Type: provider.ContentFile, Data: &provider.DataContent{URL: "https://example.com/file"}, MediaType: "image/png", ProviderMetadata: metadata}},
			want:    []provider.StreamPart{{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "https://example.com/file"}, MediaType: "image/png", ProviderMetadata: metadata}},
		},
		{
			name:    "reasoning file bytes",
			content: []provider.GenerateContentPart{{Type: provider.ContentReasoningFile, Data: &provider.DataContent{Bytes: []byte{1, 2}}, MediaType: "image/png"}},
			want:    []provider.StreamPart{{Type: provider.PartReasoningFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte{1, 2}}, MediaType: "image/png"}},
		},
		{
			name:    "reasoning file base64",
			content: []provider.GenerateContentPart{{Type: provider.ContentReasoningFile, Data: &provider.DataContent{Base64: "AQID"}, MediaType: "image/png", ProviderMetadata: metadata}},
			want:    []provider.StreamPart{{Type: provider.PartReasoningFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Base64: "AQID"}, MediaType: "image/png", ProviderMetadata: metadata}},
		},
		{
			name:    "reasoning file empty bytes",
			content: []provider.GenerateContentPart{{Type: provider.ContentReasoningFile, Data: &provider.DataContent{Bytes: []byte{}}, MediaType: "image/png"}},
			want:    []provider.StreamPart{{Type: provider.PartReasoningFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte{}}, MediaType: "image/png"}},
		},
		{
			name:    "reasoning file URL",
			content: []provider.GenerateContentPart{{Type: provider.ContentReasoningFile, Data: &provider.DataContent{URL: "https://example.com/reasoning"}, MediaType: "image/png"}},
			want:    []provider.StreamPart{{Type: provider.PartReasoningFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "https://example.com/reasoning"}, MediaType: "image/png"}},
		},
		{
			name:    "custom content",
			content: []provider.GenerateContentPart{{Type: provider.ContentCustom, Kind: "custom-kind", ProviderMetadata: metadata}},
			want:    []provider.StreamPart{{Type: provider.PartCustom, Kind: "custom-kind", ProviderMetadata: metadata}},
		},
		{
			name: "mixed ordering and IDs",
			content: []provider.GenerateContentPart{
				{Type: provider.ContentText, Text: "first"},
				{Type: provider.ContentCustom, Kind: "middle"},
				{Type: provider.ContentReasoning, Text: "think"},
				{Type: provider.ContentText},
				{Type: provider.ContentText, Text: "last"},
			},
			want: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "0"},
				{Type: provider.PartTextDelta, ID: "0", Delta: "first"},
				{Type: provider.PartTextEnd, ID: "0"},
				{Type: provider.PartCustom, Kind: "middle"},
				{Type: provider.PartReasoningStart, ID: "1"},
				{Type: provider.PartReasoningDelta, ID: "1", Delta: "think"},
				{Type: provider.PartReasoningEnd, ID: "1"},
				{Type: provider.PartTextStart, ID: "2"},
				{Type: provider.PartTextDelta, ID: "2", Delta: "last"},
				{Type: provider.PartTextEnd, ID: "2"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := &mockModel{
				doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{
						Content: tc.content, FinishReason: finish, Usage: usage,
						Warnings: warnings, ProviderMetadata: metadata,
					}, nil
				},
			}
			result, err := WrapLanguageModel(model, SimulateStreaming()).DoStream(t.Context(), provider.CallOptions{})
			require.NoError(t, err)
			var parts []provider.StreamPart
			for part := range result.Stream {
				parts = append(parts, part)
			}
			want := []provider.StreamPart{
				{Type: provider.PartStreamStart, Warnings: warnings},
				{Type: provider.PartResponseMeta},
			}
			want = append(want, tc.want...)
			want = append(want, provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish, Usage: &usage, ProviderMetadata: metadata})
			assert.Equal(t, want, parts)
			if tc.sourceJSON != "" {
				require.Greater(t, len(parts), 2)
				data, err := json.Marshal(parts[2])
				require.NoError(t, err)
				assert.JSONEq(t, tc.sourceJSON, string(data))
			}
		})
	}
}

func TestSimulateStreaming_BlockedConsumerCancellation(t *testing.T) {
	const contentCount = 128
	tests := []struct {
		name    string
		content provider.GenerateContentPart
	}{
		{"default parts", provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "test"}},
		{"text multipart", provider.GenerateContentPart{Type: provider.ContentText, Text: "hello"}},
		{"reasoning multipart", provider.GenerateContentPart{Type: provider.ContentReasoning, Text: "thinking"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content := make([]provider.GenerateContentPart, contentCount)
			for i := range content {
				content[i] = tc.content
			}
			model := &mockModel{
				doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{Content: content, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
				},
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result, err := WrapLanguageModel(model, SimulateStreaming()).DoStream(ctx, provider.CallOptions{})
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				return len(result.Stream) == cap(result.Stream)
			}, time.Second, time.Millisecond)
			cancel()

			deadline, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			var parts []provider.StreamPart
			for {
				select {
				case part, ok := <-result.Stream:
					if !ok {
						assert.Less(t, len(parts), contentCount)
						for _, received := range parts {
							assert.NotEqual(t, provider.PartFinish, received.Type)
						}
						return
					}
					parts = append(parts, part)
				case <-deadline.Done():
					t.Fatal("simulated stream did not close after cancellation")
				}
			}
		})
	}
}
