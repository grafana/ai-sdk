package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadataFits_OriginalBytesAndCardinality(t *testing.T) {
	many := make(provider.ProviderMetadata)
	for i := range 100 {
		many[strconv.Itoa(i)] = json.RawMessage(`{}`)
	}
	for _, metadata := range []provider.ProviderMetadata{
		{}, {"": json.RawMessage(`{}`)},
		{strings.Repeat("key", 100): json.RawMessage("  {\"value\":0}  ")}, many,
	} {
		budget := int64(2)
		for key, raw := range metadata {
			budget += int64(len(key) + len(raw) + 4)
		}
		if len(metadata) > 0 {
			budget--
		}
		for _, delta := range []int64{-1, 0, 1} {
			remaining := budget + delta
			assert.Equal(t, delta >= 0, metadataFits(metadata, &remaining))
			if delta >= 0 {
				assert.Equal(t, delta, remaining)
			}
		}
	}
	remaining := int64(1)
	assert.False(t, metadataFits(many, &remaining))
	assert.True(t, metadataFits(nil, &remaining))
}

func TestProviderMetadata_AggregateAndEncodingBounds(t *testing.T) {
	raw := json.RawMessage(`{"value":"` + strings.Repeat("x", 300) + `"}`)
	metadata := provider.ProviderMetadata{"future": raw}
	result := validGenerateResult()
	result.Content[0].ProviderMetadata = metadata
	result.ProviderMetadata = metadata
	_, err := mapUnarySuccess(result, 500)
	require.Error(t, err)
	result.ProviderMetadata = nil
	mapped, err := mapUnarySuccess(result, 500)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	for _, delta := range []int64{-1, 0, 1} {
		mapped, err := mapUnarySuccess(result, int64(len(body))+delta)
		require.NoError(t, err)
		_, ok := encodeUnarySuccess(mapped, int64(len(body))+delta)
		assert.Equal(t, delta >= 0, ok)
	}
	whitespace := validGenerateResult()
	whitespace.ProviderMetadata = provider.ProviderMetadata{"future": json.RawMessage(strings.Repeat(" ", 501) + `{}`)}
	assert.False(t, unarySuccessPreflight(whitespace, 500))
	event := streamEvent{typeName: provider.PartToolResult, id: "call", toolName: "weather", result: json.RawMessage(`{"value":"` + strings.Repeat("x", 300) + `"}`), metadata: metadata}
	assert.False(t, streamEventPreflight(event, 500))
	event = streamEvent{typeName: provider.PartTextDelta, id: "text", delta: "<>&", metadata: provider.ProviderMetadata{"<future>": json.RawMessage(`{"value":"<>&","surrogate":"\ud800"}`)}}
	frame, ok := encodeStreamFrame(event, 4096)
	require.True(t, ok)
	for _, delta := range []int64{-1, 0, 1} {
		got, ok := encodeStreamFrame(event, int64(len(frame))+delta)
		assert.Equal(t, delta >= 0, ok)
		if delta < 0 {
			assert.Empty(t, got)
		}
	}
	source := provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "native", URL: "https://example.test", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"opaque":"` + strings.Repeat("x", 8193) + `"}`)}}
	_, err = mapSource(source, make(sourceIDs), 1<<16)
	require.NoError(t, err)
}

func TestProviderMetadata_UTF8AndSurrogates(t *testing.T) {
	for _, metadata := range []provider.ProviderMetadata{
		{string([]byte{255}): json.RawMessage(`{}`)},
		{"future": json.RawMessage{'{', '"', 255, '"', ':', '0', '}'}},
		{"future": json.RawMessage{'{', '"', 'v', '"', ':', '"', 255, '"', '}'}},
	} {
		result := validGenerateResult()
		result.ProviderMetadata = metadata
		_, err := mapUnarySuccess(result, 4096)
		require.Error(t, err)
		frame, ok := encodeStreamFrame(streamEvent{typeName: provider.PartTextStart, id: "text", metadata: metadata}, 4096)
		assert.False(t, ok)
		assert.Empty(t, frame)
	}
	result := validGenerateResult()
	result.ProviderMetadata = provider.ProviderMetadata{"future": json.RawMessage(`{"lone":"\ud800","paired":"\ud83d\ude00"}`)}
	mapped, err := mapUnarySuccess(result, 4096)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 4096)
	require.True(t, ok)
	assert.Contains(t, string(body), `\ud800`)
	assert.Contains(t, string(body), `\ud83d\ude00`)
}

func TestProviderMetadata_RuntimeFailureAndFinishAuthority(t *testing.T) {
	for _, metadata := range []provider.ProviderMetadata{{"future": json.RawMessage(`null`)}, {"future": json.RawMessage(strings.Repeat(" ", 1<<20) + `{}`)}} {
		h := newRuntimeHarness(t, testLimits())
		h.model.generate = func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			result := validGenerateResult()
			result.ProviderMetadata = metadata
			return result, nil
		}
		response := h.serve(validRequest(`{"prompt":[]}`))
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), "future")
		h.model.stream = func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartTextStart, ID: "text", ProviderMetadata: metadata}, finishPart())}, nil
		}
		response = h.serve(streamRequest(`{"prompt":[]}`))
		assert.NotContains(t, response.Body.String(), `"type":"text-start"`)
		assert.Equal(t, 1, strings.Count(response.Body.String(), `"type":"error"`))
		assert.NotContains(t, response.Body.String(), "future")
	}
	h := newRuntimeHarness(t, testLimits())
	finish := finishPart()
	finish.ProviderMetadata = provider.ProviderMetadata{"future": json.RawMessage(`{"final":true}`)}
	h.model.stream = func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(finish, provider.StreamPart{Type: provider.PartTextStart, ID: "late", ProviderMetadata: provider.ProviderMetadata{"future": json.RawMessage(`null`)}})}, nil
	}
	body := h.serve(streamRequest(`{"prompt":[]}`)).Body.String()
	assert.Contains(t, body, `"future":{"final":true}`)
	assert.NotContains(t, body, `"type":"error"`)
	assert.NotContains(t, body, "late")
}
