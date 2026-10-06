package v4

import (
	"context"
	"encoding/json"
	"net/http"
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

func TestToolMetadata_OpaqueTransport(t *testing.T) {
	metadata := provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"caller":{"type":"code_execution_20260120","toolId":"parent","secret":"private"},"secret":"private"}`),
		"openai":    json.RawMessage(`{"itemId":"item-1","namespace":"tools","caller":{"type":"program","callerId":"parent","private":"hidden"},"backendModel":"private"}`),
		"private":   json.RawMessage(`{"credential":"private"}`),
	}
	mapped, err := mapToolMetadataForTest(metadata, 1024)
	require.NoError(t, err)
	require.Len(t, mapped, 3)
	assert.Equal(t, metadata, mapped)
}

func TestToolMetadata_OptionalFieldsAndExtensions(t *testing.T) {
	metadata := provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"type":"other","caller":{"type":"direct","callerId":"private"},"credential":"private"}`),
		"openai":    json.RawMessage(`{"itemId":"","namespace":"","caller":{"type":"direct","toolId":"private"},"credential":"private"}`),
		"azure":     json.RawMessage(`{"private":"hidden"}`),
	}
	mapped, err := mapToolMetadataForTest(metadata, 1024)
	require.NoError(t, err)
	require.Len(t, mapped, 3)
	assert.Equal(t, metadata, mapped)
}

func TestToolMetadata_OpaqueSemanticFields(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata provider.ProviderMetadata
		limit    int64
	}{
		{name: "unsupported MCP metadata", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"other"}`)}, limit: 1024},
		{name: "unsupported caller", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"unknown"}}`)}, limit: 1024},
		{name: "null caller", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":null}`)}, limit: 1024},
		{name: "null type", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":null}`)}, limit: 1024},
		{name: "missing anthropic tool id", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"code_execution_20260120"}}`)}, limit: 1024},
		{name: "null anthropic tool id", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"code_execution_20260120","toolId":null}}`)}, limit: 1024},
		{name: "direct caller with null id", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"direct","toolId":null}}`)}, limit: 1024},
		{name: "empty anthropic tool id", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"code_execution_20260120","toolId":""}}`)}, limit: 1024},
		{name: "missing openai caller id", metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"caller":{"type":"program"}}`)}, limit: 1024},
		{name: "empty azure caller id", metadata: provider.ProviderMetadata{"azure": json.RawMessage(`{"caller":{"type":"program","callerId":""}}`)}, limit: 1024},
		{name: "wrong item id type", metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":42}`)}, limit: 1024},
		{name: "null item id", metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":null}`)}, limit: 1024},
		{name: "null namespace", metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"namespace":null}`)}, limit: 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapped, err := mapToolMetadataForTest(tc.metadata, tc.limit)
			require.NoError(t, err)
			assert.Equal(t, tc.metadata, mapped)
		})
	}
}

func TestToolMetadata_StructuralAndEncodingBounds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata provider.ProviderMetadata
		limit    int64
	}{
		{name: "null known namespace", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`null`)}, limit: 1024},
		{name: "encoded response exceeds limit", metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"<&>"}`)}, limit: 25},
		{name: "malformed metadata", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{`)}, limit: 1024},
		{name: "oversized known field", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"` + strings.Repeat("x", 200) + `"}`)}, limit: 128},
		{name: "oversized unknown namespace", metadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"` + strings.Repeat("x", 200) + `"}`)}, limit: 128},
		{name: "excess cardinality", metadata: provider.ProviderMetadata{"one": nil, "two": nil}, limit: 1},
		{name: "array namespace", metadata: provider.ProviderMetadata{"future": json.RawMessage(`[]`)}, limit: 1024},
		{name: "invalid UTF-8 namespace", metadata: provider.ProviderMetadata{"future": json.RawMessage([]byte{'{', '"', 0xff, '"', ':', '0', '}'})}, limit: 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mapToolMetadataForTest(tc.metadata, tc.limit)
			require.Error(t, err)
		})
	}
}

func mapToolMetadataForTest(metadata provider.ProviderMetadata, limit int64) (provider.ProviderMetadata, error) {
	result := validGenerateResult()
	result.Content = []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage("{}"), ProviderMetadata: metadata}}
	mapped, err := mapUnarySuccess(result, limit)
	if err != nil {
		return nil, err
	}
	body, ok := encodeUnarySuccess(mapped, limit)
	if !ok {
		return nil, errInvalidUnarySuccess
	}
	var response struct {
		Content []struct {
			Metadata provider.ProviderMetadata `json:"providerMetadata"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	return response.Content[0].Metadata, nil
}

func TestRuntimeToolMetadata_OpaquePresence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata provider.ProviderMetadata
	}{
		{name: "omitted"},
		{name: "empty", metadata: provider.ProviderMetadata{}},
		{name: "inert semantic fields", metadata: provider.ProviderMetadata{
			"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"unconfigured","caller":null}`),
			"openai":    json.RawMessage(`{"itemId":42,"namespace":null,"caller":{"type":"future"}}`),
		}},
		{name: "extensions", metadata: provider.ProviderMetadata{
			"anthropic": json.RawMessage(`{"caller":{"type":"direct","extension":{"nested":[null,false,{}]}}}`),
			"future":    json.RawMessage(`{"credential":"opaque-not-a-routing-authority","nested":{"unicode":"☃<&>"}}`),
		}},
	} {
		for _, streaming := range []bool{false, true} {
			name := tc.name + "/unary"
			if streaming {
				name = tc.name + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					result := validGenerateResult()
					result.Content = []provider.GenerateContentPart{
						{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage(`{}`), ProviderExecuted: true, Dynamic: new(false), ProviderMetadata: tc.metadata},
						{Type: provider.ContentToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`), Dynamic: new(false), Preliminary: new(false), ProviderMetadata: tc.metadata},
					}
					return result, nil
				}
				harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: makeStream(
						provider.StreamPart{Type: provider.PartToolInputStart, ID: "call", ToolName: "echo", ProviderExecuted: true, Dynamic: new(false), ProviderMetadata: tc.metadata},
						provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call", Delta: "{}", ProviderMetadata: tc.metadata},
						provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call", ProviderMetadata: tc.metadata},
						provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "echo", Input: "{}", ProviderExecuted: true, Dynamic: new(false), ProviderMetadata: tc.metadata},
						provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`), Dynamic: new(false), Preliminary: new(false), ProviderMetadata: tc.metadata},
						finishPart(),
					)}, nil
				}
				request := validRequest(`{"prompt":[]}`)
				if streaming {
					request.Header.Set(HeaderStreaming, "true")
				}
				response := harness.serve(request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var parts []json.RawMessage
				if streaming {
					requireStreamBodyMatchesSchema(t, response.Body.String())
					for _, frame := range strings.Split(response.Body.String(), "\n\n") {
						if strings.Contains(frame, `"type":"tool-`) {
							parts = append(parts, json.RawMessage(strings.TrimPrefix(frame, "data: ")))
						}
					}
					require.Len(t, parts, 5)
				} else {
					var body struct {
						Content []json.RawMessage `json:"content"`
					}
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
					parts = body.Content
					require.Len(t, parts, 2)
				}
				for _, part := range parts {
					var fields map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(part, &fields))
					metadata, exists := fields["providerMetadata"]
					assert.Equal(t, tc.metadata != nil, exists)
					if tc.metadata != nil {
						want, err := json.Marshal(tc.metadata)
						require.NoError(t, err)
						assert.JSONEq(t, string(want), string(metadata))
					}
					if string(fields["type"]) == `"tool-call"` || string(fields["type"]) == `"tool-result"` {
						assert.Equal(t, "false", string(fields["dynamic"]))
					}
					if string(fields["type"]) == `"tool-result"` {
						assert.Equal(t, "false", string(fields["preliminary"]))
					}
				}
			})
		}
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
