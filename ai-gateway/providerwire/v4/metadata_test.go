package v4

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderMetadata_UnaryScopes(t *testing.T) {
	file := provider.Base64DataContent("")
	for _, metadata := range []provider.ProviderMetadata{nil, {}, {"future": json.RawMessage(`{"nested":[null,false,0,"",[],{}],"token":"sk-application-data"}`)}} {
		result := validGenerateResult()
		result.ProviderMetadata = metadata
		result.Content = []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "hello", ProviderMetadata: metadata},
			{Type: provider.ContentReasoning, Text: "", ProviderMetadata: metadata},
			{Type: provider.ContentReasoningFile, Data: &file, MediaType: "image/png", ProviderMetadata: metadata},
			{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: "native", URL: "https://example.test", ProviderMetadata: metadata},
			{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "weather", Input: json.RawMessage(`{}`), ProviderMetadata: metadata},
		}
		mapped, err := mapUnarySuccess(result, 1<<20)
		require.NoError(t, err)
		body, ok := encodeUnarySuccess(mapped, 1<<20)
		require.True(t, ok)
		compiled, err := schema.CompileSchema(unarySuccessSchemaJSON)
		require.NoError(t, err)
		require.NoError(t, compiled.Validate(body))
		var decoded struct {
			Content  []map[string]json.RawMessage
			Metadata json.RawMessage `json:"providerMetadata"`
		}
		require.NoError(t, json.Unmarshal(body, &decoded))
		checkMetadataPresence(t, metadata, decoded.Metadata)
		for _, part := range decoded.Content {
			checkMetadataPresence(t, metadata, part["providerMetadata"])
		}
	}
}

func checkMetadataPresence(t *testing.T, want provider.ProviderMetadata, raw json.RawMessage) {
	t.Helper()
	if want == nil {
		assert.Empty(t, raw)
		return
	}
	require.NotEmpty(t, raw)
	encoded, err := json.Marshal(want)
	require.NoError(t, err)
	assert.JSONEq(t, string(encoded), string(raw))
}

func TestProviderMetadata_StreamScopes(t *testing.T) {
	for _, metadata := range []provider.ProviderMetadata{nil, {}, {"future": json.RawMessage(`{"nested":[null,false,0,"",[],{}]}`)}} {
		finish := finishPart()
		finish.ProviderMetadata = metadata
		parts := []provider.StreamPart{
			{Type: provider.PartTextStart, ID: "text", ProviderMetadata: metadata},
			{Type: provider.PartTextDelta, ID: "text", Delta: "", ProviderMetadata: metadata},
			{Type: provider.PartTextEnd, ID: "text", ProviderMetadata: metadata},
			{Type: provider.PartReasoningStart, ID: "reasoning", ProviderMetadata: metadata},
			{Type: provider.PartReasoningDelta, ID: "reasoning", Delta: "", ProviderMetadata: metadata},
			{Type: provider.PartReasoningEnd, ID: "reasoning", ProviderMetadata: metadata},
			{Type: provider.PartReasoningFile, MediaType: "image/png", Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData}, ProviderMetadata: metadata},
			{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "native", URL: "https://example.test", ProviderMetadata: metadata}},
			{Type: provider.PartToolInputStart, ID: "call", ToolName: "weather", ProviderMetadata: metadata},
			{Type: provider.PartToolInputDelta, ID: "call", Delta: "", ProviderMetadata: metadata},
			{Type: provider.PartToolInputEnd, ID: "call", ProviderMetadata: metadata},
			{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "weather", Input: "{}", ProviderMetadata: metadata},
			{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "weather", Result: json.RawMessage(`{}`), ProviderMetadata: metadata},
			finish,
		}
		h := &handler{limits: Limits{StreamFrameBytes: 1 << 20, StreamParts: 100}}
		state := newStreamState(100, nil)
		w := httptest.NewRecorder()
		for i, part := range parts {
			before := w.Body.Len()
			want := streamPartContinue
			if i == len(parts)-1 {
				want = streamPartFinished
			}
			require.Equal(t, want, h.processStreamPart(w, state, part), part.Type)
			var decoded map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(w.Body.String()[before:], "data: "), "\n\n")), &decoded))
			checkMetadataPresence(t, metadata, decoded["providerMetadata"])
		}
		requireStreamBodyMatchesSchema(t, w.Body.String())
	}
}

func TestProviderMetadata_InvalidUnary(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`true`), json.RawMessage(`{"incomplete":`), json.RawMessage{'{', '"', 255, '"', ':', '0', '}'}} {
		for _, scope := range []string{"result", "text", "reasoning", "source", "tool"} {
			t.Run(scope+"/"+string(raw), func(t *testing.T) {
				result := validGenerateResult()
				metadata := provider.ProviderMetadata{"future": raw}
				switch scope {
				case "result":
					result.ProviderMetadata = metadata
				case "text":
					result.Content[0].ProviderMetadata = metadata
				case "reasoning":
					result.Content = []provider.GenerateContentPart{{Type: provider.ContentReasoning, Text: "thought", ProviderMetadata: metadata}}
				case "source":
					result.Content = []provider.GenerateContentPart{{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: "native", URL: "https://example.test", ProviderMetadata: metadata}}
				case "tool":
					result.Content = []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "weather", Input: json.RawMessage(`{}`), ProviderMetadata: metadata}}
				}
				w := httptest.NewRecorder()
				h := &handler{limits: Limits{UnaryResponseBytes: 1 << 20}}
				assert.False(t, h.writeUnarySuccess(w, result))
				assert.Empty(t, w.Body.String())
			})
		}
	}
}

func TestProviderMetadata_OriginalBytesAndCardinality(t *testing.T) {
	many := make(provider.ProviderMetadata)
	for i := range 100 {
		many[strconv.Itoa(i)] = json.RawMessage("{}")
	}
	for _, tc := range []struct {
		name     string
		metadata provider.ProviderMetadata
	}{
		{name: "omitted"},
		{name: "empty", metadata: provider.ProviderMetadata{}},
		{name: "empty namespace", metadata: provider.ProviderMetadata{"": json.RawMessage("{}")}},
		{name: "long key and whitespace", metadata: provider.ProviderMetadata{strings.Repeat("key", 100): json.RawMessage("  {\"value\":0}  ")}},
		{name: "many namespaces", metadata: many},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget := int64(len(tc.metadata))
			for key, raw := range tc.metadata {
				budget += int64(len(key)) + int64(len(raw))
			}
			for _, delta := range []int64{-1, 0, 1} {
				limit := budget + delta + int64(len("native"))
				result := validGenerateResult()
				result.Content = nil
				result.FinishReason.Raw = "native"
				result.ProviderMetadata = tc.metadata
				assert.Equal(t, delta >= 0, unarySuccessPreflight(result, limit))

				event := streamEvent{typeName: provider.PartTextStart, id: "native", metadata: tc.metadata}
				assert.Equal(t, delta >= 0, streamEventPreflight(event, limit+int64(len(provider.PartTextStart))))

				source := provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "native", ProviderMetadata: tc.metadata}
				assert.Equal(t, delta >= 0, sourcePreflight(source, limit))
			}
		})
	}
	result := validGenerateResult()
	result.ProviderMetadata = many
	assert.False(t, unarySuccessPreflight(result, 1))
	assert.False(t, streamEventPreflight(streamEvent{typeName: provider.PartTextStart, metadata: many}, 1))
	assert.False(t, sourcePreflight(provider.SourceInfo{ID: "native", ProviderMetadata: many}, 1))
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
	_, err = mapSource(source, 1<<16)
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
