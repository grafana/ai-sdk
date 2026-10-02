package v4

import (
	"encoding/json"
	"net/http/httptest"
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
		state := newStreamState(100)
		w := httptest.NewRecorder()
		for i, part := range parts {
			before := w.Body.Len()
			want := streamPartContinue
			if i == len(parts)-1 {
				want = streamPartFinished
			}
			require.Equal(t, want, h.processStreamPart(w, state, part, "public/model"), part.Type)
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
				_, err := mapUnarySuccess(result, 1<<20)
				require.Error(t, err)
			})
		}
	}
}
