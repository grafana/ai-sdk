package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func customResult(parts ...provider.GenerateContentPart) *provider.GenerateResult {
	return &provider.GenerateResult{Content: parts, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}
}

func TestUnaryCustomContent_Output(t *testing.T) {
	opaque := json.RawMessage(`{"itemId":"cmp_1","nested":{"zero":0,"no":false,"none":null,"empty":"","list":[]}}`)
	tests := []struct {
		name string
		part provider.GenerateContentPart
		want string
	}{
		{"absent metadata", provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "anthropic.fallback"}, `{"type":"custom","kind":"anthropic.fallback"}`},
		{"empty metadata", provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "example.kind", ProviderMetadata: provider.ProviderMetadata{}}, `{"type":"custom","kind":"example.kind","providerMetadata":{}}`},
		{"opaque metadata", provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "openai.compaction", ProviderMetadata: provider.ProviderMetadata{"openai": opaque}}, `{"type":"custom","kind":"openai.compaction","providerMetadata":{"openai":` + string(opaque) + `}}`},
	}
	for _, tc := range tests {
		t.Run("custom-only "+tc.name, func(t *testing.T) {
			mapped, err := mapUnarySuccess(customResult(tc.part), 1<<20)
			require.NoError(t, err)
			body, ok := encodeUnarySuccess(mapped, 1<<20)
			require.True(t, ok)
			require.NoError(t, compileWireSchema(t, unarySuccessSchemaJSON).Validate(body))
			var decoded struct {
				Content []json.RawMessage `json:"content"`
			}
			require.NoError(t, json.Unmarshal(body, &decoded))
			require.Len(t, decoded.Content, 1)
			assert.JSONEq(t, tc.want, string(decoded.Content[0]))
		})
	}

	t.Run("keeps content order", func(t *testing.T) {
		mapped, err := mapUnarySuccess(customResult(
			provider.GenerateContentPart{Type: provider.ContentText, Text: "before"},
			provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "example.kind"},
			provider.GenerateContentPart{Type: provider.ContentText, Text: "after"},
		), 1<<20)
		require.NoError(t, err)
		body, ok := encodeUnarySuccess(mapped, 1<<20)
		require.True(t, ok)
		var decoded struct {
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		}
		require.NoError(t, json.Unmarshal(body, &decoded))
		require.Len(t, decoded.Content, 3)
		assert.Equal(t, []string{"text", "custom", "text"}, []string{decoded.Content[0].Type, decoded.Content[1].Type, decoded.Content[2].Type})
	})

	t.Run("metadata budget is shared", func(t *testing.T) {
		part := provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "example.kind", ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"` + strings.Repeat("x", 1000) + `"}`)}}
		_, err := mapUnarySuccess(customResult(part), 512)
		require.Error(t, err)
		_, err = mapUnarySuccess(customResult(part), 1<<20)
		require.NoError(t, err)
	})

	t.Run("kind bytes count toward the preflight", func(t *testing.T) {
		part := provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "example." + strings.Repeat("k", 4096)}
		_, err := mapUnarySuccess(customResult(part), 2048)
		require.Error(t, err)
	})

	t.Run("invalid kinds and metadata fail before HTTP 200", func(t *testing.T) {
		for name, part := range map[string]provider.GenerateContentPart{
			"empty kind":       {Type: provider.ContentCustom},
			"undotted kind":    {Type: provider.ContentCustom, Kind: "private"},
			"invalid UTF-8":    {Type: provider.ContentCustom, Kind: "private." + string([]byte{0xff})},
			"array metadata":   {Type: provider.ContentCustom, Kind: "example.kind", ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`[]`)}},
			"invalid metadata": {Type: provider.ContentCustom, Kind: "example.kind", ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{`)}},
		} {
			t.Run(name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return customResult(part), nil
				}

				response := harness.serve(validRequest(`{"prompt":[]}`))

				assert.Equal(t, http.StatusInternalServerError, response.Code)
				assert.JSONEq(t, string(canonicalInternalError), response.Body.String())
				assert.NotContains(t, response.Body.String(), "private")
			})
		}
	})

	t.Run("custom-only success is served once", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return customResult(provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "anthropic.fallback", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"fallback"}`)}}), nil
		}

		response := harness.serve(validRequest(`{"prompt":[]}`))

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Equal(t, 1, harness.model.callCount())
		assert.Contains(t, response.Body.String(), `"kind":"anthropic.fallback"`)
	})
}

func TestStreamCustomContent_Output(t *testing.T) {
	custom := func(kind string, metadata provider.ProviderMetadata) provider.StreamPart {
		return provider.StreamPart{Type: provider.PartCustom, Kind: kind, ProviderMetadata: metadata}
	}
	serve := func(t *testing.T, parts ...provider.StreamPart) string {
		t.Helper()
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(parts...)}, nil
		}
		response := harness.serve(streamRequest(`{"prompt":[]}`))
		require.Equal(t, http.StatusOK, response.Code)
		return response.Body.String()
	}

	t.Run("custom between open text and tool blocks", func(t *testing.T) {
		body := serve(t,
			provider.StreamPart{Type: provider.PartTextStart, ID: "a"},
			provider.StreamPart{Type: provider.PartToolInputStart, ID: "call", ToolName: "lookup"},
			custom("openai.compaction", provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"cmp_1"}`)}),
			provider.StreamPart{Type: provider.PartTextDelta, ID: "a", Delta: "x"},
			provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call", Delta: "{}"},
			custom("example.kind", nil),
			provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call"},
			provider.StreamPart{Type: provider.PartTextEnd, ID: "a"},
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: "{}"},
			finishPart(),
		)
		requireStreamBodyMatchesSchema(t, body)
		assert.NotContains(t, body, `"code":"internal_error"`)
		var types []string
		for _, frame := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
			var event struct {
				Type string `json:"type"`
			}
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &event))
			types = append(types, event.Type)
		}
		assert.Equal(t, []string{"stream-start", "text-start", "tool-input-start", "custom", "text-delta", "tool-input-delta", "custom", "tool-input-end", "text-end", "tool-call", "finish"}, types)
		assert.Contains(t, body, `data: {"type":"custom","kind":"openai.compaction","providerMetadata":{"openai":{"itemId":"cmp_1"}}}`)
		assert.Contains(t, body, `data: {"type":"custom","kind":"example.kind"}`)
	})

	t.Run("metadata presence", func(t *testing.T) {
		body := serve(t, custom("example.empty", provider.ProviderMetadata{}), custom("example.bare", nil), finishPart())
		requireStreamBodyMatchesSchema(t, body)
		assert.Contains(t, body, `data: {"type":"custom","kind":"example.empty","providerMetadata":{}}`)
		assert.Contains(t, body, `data: {"type":"custom","kind":"example.bare"}`)
	})

	t.Run("response metadata after custom fails", func(t *testing.T) {
		body := serve(t, custom("example.kind", nil), provider.StreamPart{Type: provider.PartResponseMeta, ModelID: "m"}, finishPart())
		assert.Equal(t, 1, strings.Count(body, `"code":"internal_error"`))
		assert.NotContains(t, body, `"type":"response-metadata"`)
		assert.NotContains(t, body, `"type":"finish"`)
	})

	t.Run("synthetic message_start shape after response metadata", func(t *testing.T) {
		start := custom("anthropic.message_start", provider.ProviderMetadata{"anthropic": json.RawMessage(`{"id":"msg_1","model":"claude","usage":{"input_tokens":3}}`)})
		body := serve(t,
			provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "msg_1", ModelID: "claude"},
			start,
			provider.StreamPart{Type: provider.PartTextStart, ID: "a"},
			provider.StreamPart{Type: provider.PartTextDelta, ID: "a", Delta: "hi"},
			provider.StreamPart{Type: provider.PartTextEnd, ID: "a"},
			finishPart(),
		)
		requireStreamBodyMatchesSchema(t, body)
		assert.NotContains(t, body, `"code":"internal_error"`)
		metadata := strings.Index(body, `"type":"response-metadata"`)
		message := strings.Index(body, `"kind":"anthropic.message_start"`)
		text := strings.Index(body, `"type":"text-start"`)
		assert.True(t, metadata >= 0 && metadata < message && message < text, body)
		assert.Contains(t, body, `"providerMetadata":{"anthropic":{"id":"msg_1","model":"claude","usage":{"input_tokens":3}}}`)
	})

	t.Run("oversized frame produces one terminal error", func(t *testing.T) {
		limits := testLimits()
		limits.StreamFrameBytes = 256
		harness := newRuntimeHarness(t, limits)
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(
				custom("example.kind", provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"` + strings.Repeat("x", 1000) + `"}`)}),
				finishPart(),
			)}, nil
		}

		response := harness.serve(streamRequest(`{"prompt":[]}`))

		assert.Equal(t, 1, strings.Count(response.Body.String(), `"code":"internal_error"`))
		assert.NotContains(t, response.Body.String(), "secret")
		assert.NotContains(t, response.Body.String(), `"type":"finish"`)
	})
}
