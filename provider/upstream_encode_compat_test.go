package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests lock the provider-domain JSON representation: encoders emit the
// upstream Vercel AI SDK LanguageModelV4 shapes, and decoders accept both the
// upstream and legacy Go encodings.

func TestDataContent_MarshalUpstreamUnion(t *testing.T) {
	cases := []struct {
		name string
		in   DataContent
		want string
	}{
		{"bytes", DataContent{Bytes: []byte{0x01, 0x02, 0x03}}, `{"type":"data","data":"AQID"}`},
		{"empty bytes", DataContent{Bytes: []byte{}}, `{"type":"data","data":""}`},
		{"base64", DataContent{Base64: "aGVsbG8="}, `{"type":"data","data":"aGVsbG8="}`},
		{"url", DataContent{URL: "https://example.com/x.png"}, `{"type":"url","url":"https://example.com/x.png"}`},
		{"reference", DataContent{Reference: json.RawMessage(`{"openai":"file_123"}`)}, `{"type":"reference","reference":{"openai":"file_123"}}`},
		{"text", DataContent{Text: "inline doc"}, `{"type":"text","text":"inline doc"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.in)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(got))
		})
	}
}

func TestDataContent_DecodeReferenceAndText(t *testing.T) {
	var ref DataContent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"reference","reference":{"openai":"file_123"}}`), &ref))
	assert.JSONEq(t, `{"openai":"file_123"}`, string(ref.Reference))

	var text DataContent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"text","text":"inline doc"}`), &text))
	assert.Equal(t, "inline doc", text.Text)
}

func TestGenerateContentPart_ToolCallInputString(t *testing.T) {
	part := GenerateContentPart{
		Type:       ContentToolCall,
		ToolCallID: "call_1",
		ToolName:   "search",
		Input:      json.RawMessage(`{"query":"grafana"}`),
	}
	got, err := json.Marshal(part)
	require.NoError(t, err)
	// input is emitted as a stringified JSON string.
	assert.JSONEq(t, `{"type":"tool-call","toolCallId":"call_1","toolName":"search","input":"{\"query\":\"grafana\"}"}`, string(got))

	// Decode tolerates both the string and the raw-object forms.
	var fromString GenerateContentPart
	require.NoError(t, json.Unmarshal(got, &fromString))
	assert.JSONEq(t, `{"query":"grafana"}`, string(fromString.Input))

	var fromObject GenerateContentPart
	require.NoError(t, json.Unmarshal([]byte(`{"type":"tool-call","toolCallId":"call_1","toolName":"search","input":{"query":"grafana"}}`), &fromObject))
	assert.JSONEq(t, `{"query":"grafana"}`, string(fromObject.Input))
}

func TestGenerateContentPart_SourceTitle(t *testing.T) {
	part := GenerateContentPart{
		Type:       ContentSource,
		SourceType: SourceTypeURL,
		ID:         "src_1",
		URL:        "https://example.com",
		Title:      "Example",
	}
	got, err := json.Marshal(part)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"source","sourceType":"url","id":"src_1","url":"https://example.com","title":"Example"}`, string(got))

	var decoded GenerateContentPart
	require.NoError(t, json.Unmarshal(got, &decoded))
	assert.Equal(t, "Example", decoded.Title)
}

func TestStreamPart_MarshalUpstreamShapes(t *testing.T) {
	fileCases := []struct {
		name     string
		partType StreamPartType
		data     StreamFileData
		want     string
	}{
		{"file bytes", PartFile, StreamFileData{Type: StreamFileDataTypeData, Bytes: []byte{0x01, 0x02, 0x03}}, `{"type":"file","mediaType":"image/png","data":{"type":"data","data":"AQID"}}`},
		{"file base64", PartFile, StreamFileData{Type: StreamFileDataTypeData, Base64: "AQID"}, `{"type":"file","mediaType":"image/png","data":{"type":"data","data":"AQID"}}`},
		{"file empty data", PartFile, StreamFileData{Type: StreamFileDataTypeData}, `{"type":"file","mediaType":"image/png","data":{"type":"data","data":""}}`},
		{"file URL", PartFile, StreamFileData{Type: StreamFileDataTypeURL, URL: "https://example.com/image.png"}, `{"type":"file","mediaType":"image/png","data":{"type":"url","url":"https://example.com/image.png"}}`},
		{"reasoning-file bytes", PartReasoningFile, StreamFileData{Type: StreamFileDataTypeData, Bytes: []byte{0x01, 0x02, 0x03}}, `{"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":"AQID"}}`},
		{"reasoning-file base64", PartReasoningFile, StreamFileData{Type: StreamFileDataTypeData, Base64: "AQID"}, `{"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":"AQID"}}`},
		{"reasoning-file empty data", PartReasoningFile, StreamFileData{Type: StreamFileDataTypeData}, `{"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":""}}`},
		{"reasoning-file URL", PartReasoningFile, StreamFileData{Type: StreamFileDataTypeURL, URL: "https://example.com/image.png"}, `{"type":"reasoning-file","mediaType":"image/png","data":{"type":"url","url":"https://example.com/image.png"}}`},
	}
	for _, tc := range fileCases {
		t.Run(tc.name, func(t *testing.T) {
			p := StreamPart{Type: tc.partType, Data: &tc.data, MediaType: "image/png"}
			got, err := json.Marshal(p)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(got))
		})
	}

	t.Run("tool-result emits flat result + isError", func(t *testing.T) {
		p := StreamPart{
			Type:             PartToolResult,
			ToolCallID:       "call_1",
			ToolName:         "webSearch",
			Result:           json.RawMessage(`{"answer":"42"}`),
			IsError:          true,
			ProviderExecuted: true,
		}
		got, err := json.Marshal(p)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"tool-result","toolCallId":"call_1","toolName":"webSearch","result":{"answer":"42"},"isError":true}`, string(got))
		assert.NotContains(t, string(got), "providerExecuted")

		var decoded StreamPart
		require.NoError(t, json.Unmarshal(got, &decoded))
		assert.JSONEq(t, `{"answer":"42"}`, string(decoded.Result))
		assert.True(t, decoded.IsError)
	})

	t.Run("source emits flat fields", func(t *testing.T) {
		p := StreamPart{
			Type: PartSource,
			Source: &SourceInfo{
				SourceType: SourceTypeURL,
				ID:         "src_1",
				URL:        "https://example.com",
				Title:      "Example",
			},
		}
		got, err := json.Marshal(p)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"source","sourceType":"url","id":"src_1","url":"https://example.com","title":"Example"}`, string(got))

		var decoded StreamPart
		require.NoError(t, json.Unmarshal(got, &decoded))
		require.NotNil(t, decoded.Source)
		assert.Equal(t, SourceTypeURL, decoded.Source.SourceType)
		assert.Equal(t, "src_1", decoded.Source.ID)
		assert.Equal(t, "https://example.com", decoded.Source.URL)
		assert.Equal(t, "Example", decoded.Source.Title)
		assert.Empty(t, decoded.ID)
		assert.Empty(t, decoded.Title)
	})

	t.Run("error emits error field carrying APICallError", func(t *testing.T) {
		p := StreamPart{
			Type: PartError,
			APICallError: NewAPICallError(APICallErrorOptions{
				Message:    "boom",
				StatusCode: 500,
			}),
		}
		got, err := json.Marshal(p)
		require.NoError(t, err)
		assert.NotContains(t, string(got), "apiCallError")
		assert.Contains(t, string(got), `"type":"error"`)
		assert.Contains(t, string(got), `"message":"boom"`)

		var decoded StreamPart
		require.NoError(t, json.Unmarshal(got, &decoded))
		require.NotNil(t, decoded.APICallError)
		assert.Equal(t, "boom", decoded.APICallError.Message)
		assert.Equal(t, 500, decoded.APICallError.StatusCode)
	})

	t.Run("zero timestamp is suppressed", func(t *testing.T) {
		p := StreamPart{Type: PartTextDelta, ID: "t0", Delta: "hi"}
		got, err := json.Marshal(p)
		require.NoError(t, err)
		assert.NotContains(t, string(got), "timestamp")
	})
}

func TestStreamPart_ToolResultRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		result  json.RawMessage
		isError bool
	}{
		{name: "string", result: json.RawMessage(`"done"`)},
		{name: "object", result: json.RawMessage(`{"answer":42}`)},
		{name: "array", result: json.RawMessage(`[1,"two",true]`)},
		{name: "number", result: json.RawMessage(`42`)},
		{name: "boolean", result: json.RawMessage(`false`)},
		{name: "error", result: json.RawMessage(`{"message":"boom"}`), isError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := StreamPart{
				Type:       PartToolResult,
				ToolCallID: "call_1",
				ToolName:   "webSearch",
				Result:     tc.result,
				IsError:    tc.isError,
			}
			encoded, err := json.Marshal(original)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), `"result":`)
			assert.NotContains(t, string(encoded), `"output":`)
			assert.NotContains(t, string(encoded), "grafana-ai-sdk")

			var decoded StreamPart
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			assert.JSONEq(t, string(tc.result), string(decoded.Result))
			assert.Equal(t, tc.isError, decoded.IsError)
		})
	}
}

func TestStreamPart_ToolResultProviderMetadataIsOpaque(t *testing.T) {
	original := StreamPart{
		Type:       PartToolResult,
		ToolCallID: "call_1",
		ToolName:   "webSearch",
		Result:     json.RawMessage(`"done"`),
		ProviderMetadata: ProviderMetadata{
			"grafana-ai-sdk": json.RawMessage(`{"toolResultOutputType":"content","customer":"keep"}`),
			"anthropic":      json.RawMessage(`{"cacheControl":"ephemeral"}`),
		},
	}
	encoded, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded StreamPart
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.JSONEq(t, `"done"`, string(decoded.Result))
	require.Len(t, decoded.ProviderMetadata, 2)
	assert.JSONEq(t, string(original.ProviderMetadata["grafana-ai-sdk"]), string(decoded.ProviderMetadata["grafana-ai-sdk"]))
	assert.JSONEq(t, string(original.ProviderMetadata["anthropic"]), string(decoded.ProviderMetadata["anthropic"]))
}

// TestStreamPart_DecodeLegacyShapes proves the decoders still accept the legacy
// Go-to-Go encodings (fileData, output, nested source, apiCallError).
func TestStreamPart_DecodeLegacyShapes(t *testing.T) {
	t.Run("legacy fileData", func(t *testing.T) {
		var p StreamPart
		require.NoError(t, json.Unmarshal([]byte(`{"type":"file","fileData":"AQID","mediaType":"image/png"}`), &p))
		require.NotNil(t, p.Data)
		assert.Equal(t, StreamFileDataTypeData, p.Data.Type)
		assert.Equal(t, []byte{0x01, 0x02, 0x03}, p.Data.Bytes)
	})

	t.Run("legacy empty fileData", func(t *testing.T) {
		var p StreamPart
		require.NoError(t, json.Unmarshal([]byte(`{"type":"reasoning-file","fileData":"","mediaType":"image/png"}`), &p))
		require.NotNil(t, p.Data)
		assert.Equal(t, StreamFileDataTypeData, p.Data.Type)

		got, err := json.Marshal(p)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":""}}`, string(got))
	})

	t.Run("legacy output", func(t *testing.T) {
		cases := []struct {
			name        string
			output      string
			wantResult  string
			wantIsError bool
		}{
			{name: "text", output: `{"type":"text","text":"ok"}`, wantResult: `"ok"`},
			{name: "error text", output: `{"type":"error-text","text":"boom"}`, wantResult: `"boom"`, wantIsError: true},
			{name: "json", output: `{"type":"json","json":{"a":1}}`, wantResult: `{"a":1}`},
			{name: "error json", output: `{"type":"error-json","json":{"code":1}}`, wantResult: `{"code":1}`, wantIsError: true},
			{name: "content", output: `{"type":"content","content":[{"type":"text","text":"ok"}]}`, wantResult: `[{"type":"text","text":"ok"}]`},
			{name: "execution denied", output: `{"type":"execution-denied","reason":"nope"}`, wantResult: `"nope"`, wantIsError: true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var p StreamPart
				wire := `{"type":"tool-result","toolCallId":"c1","toolName":"t","output":` + tc.output + `}`
				require.NoError(t, json.Unmarshal([]byte(wire), &p))
				assert.JSONEq(t, tc.wantResult, string(p.Result))
				assert.Equal(t, tc.wantIsError, p.IsError)
			})
		}
	})

	t.Run("canonical result takes precedence over legacy output", func(t *testing.T) {
		var p StreamPart
		require.NoError(t, json.Unmarshal([]byte(`{"type":"tool-result","toolCallId":"c1","toolName":"t","result":false,"output":{"type":"text","text":"legacy"}}`), &p))
		assert.JSONEq(t, `false`, string(p.Result))
	})

	t.Run("legacy nested source", func(t *testing.T) {
		var p StreamPart
		require.NoError(t, json.Unmarshal([]byte(`{"type":"source","source":{"sourceType":"url","id":"s1","url":"https://x","title":"X"}}`), &p))
		require.NotNil(t, p.Source)
		assert.Equal(t, "s1", p.Source.ID)
		assert.Equal(t, "X", p.Source.Title)
	})

	t.Run("legacy apiCallError", func(t *testing.T) {
		var p StreamPart
		require.NoError(t, json.Unmarshal([]byte(`{"type":"error","apiCallError":{"message":"boom","statusCode":500,"isRetryable":true}}`), &p))
		require.NotNil(t, p.APICallError)
		assert.Equal(t, "boom", p.APICallError.Message)
		assert.True(t, p.APICallError.IsRetryable)
	})
}

// TestStreamPart_DecodeErrorPreservesDetail locks the lenient stream-decode
// policy: an upstream `error` payload that is not an APICallError-shaped object
// (a bare string or an arbitrary value) is preserved as the message rather than
// dropped, so consumers do not fall back to the generic "PartError without
// details" message.
func TestStreamPart_DecodeErrorPreservesDetail(t *testing.T) {
	t.Run("APICallError object", func(t *testing.T) {
		var p StreamPart
		require.NoError(t, json.Unmarshal([]byte(`{"type":"error","error":{"message":"boom","statusCode":500}}`), &p))
		require.NotNil(t, p.APICallError)
		assert.Equal(t, "boom", p.APICallError.Message)
		assert.Equal(t, 500, p.APICallError.StatusCode)
	})

	t.Run("bare string error", func(t *testing.T) {
		var p StreamPart
		require.NoError(t, json.Unmarshal([]byte(`{"type":"error","error":"rate limited"}`), &p))
		require.NotNil(t, p.APICallError)
		assert.Equal(t, "rate limited", p.APICallError.Message)
	})

	t.Run("non-APICallError object preserves raw detail", func(t *testing.T) {
		var p StreamPart
		require.NoError(t, json.Unmarshal([]byte(`{"type":"error","error":{"weird":"shape"}}`), &p))
		require.NotNil(t, p.APICallError)
		assert.JSONEq(t, `{"weird":"shape"}`, p.APICallError.Message)
	})
}

func TestMessage_MarshalSystemString(t *testing.T) {
	m := NewSystemMessage("be helpful")
	got, err := json.Marshal(m)
	require.NoError(t, err)
	assert.JSONEq(t, `{"role":"system","content":"be helpful"}`, string(got))

	// Non-system roles keep the array form.
	u := NewUserMessage(TextPart("hi"))
	gotU, err := json.Marshal(u)
	require.NoError(t, err)
	assert.JSONEq(t, `{"role":"user","content":[{"type":"text","text":"hi"}]}`, string(gotU))
}

func TestToolResultOutput_MarshalValueUnion(t *testing.T) {
	cases := []struct {
		name string
		in   ToolResultOutput
		want string
	}{
		{"text", ToolResultOutput{Type: ToolOutputText, Text: "ok"}, `{"type":"text","value":"ok"}`},
		{"error-text", ToolResultOutput{Type: ToolOutputErrorText, Text: "boom"}, `{"type":"error-text","value":"boom"}`},
		{"json", ToolResultOutput{Type: ToolOutputJSON, JSON: json.RawMessage(`{"a":1}`)}, `{"type":"json","value":{"a":1}}`},
		{"content", ToolResultOutput{Type: ToolOutputContent, Content: []ToolResultContentValue{{Type: ToolContentText, Text: "hi"}}}, `{"type":"content","value":[{"type":"text","text":"hi"}]}`},
		{"execution-denied", ToolResultOutput{Type: ToolOutputExecutionDenied, Reason: "nope"}, `{"type":"execution-denied","reason":"nope"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.in)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(got))

			// Round-trips back through the tolerant decoder.
			var decoded ToolResultOutput
			require.NoError(t, json.Unmarshal(got, &decoded))
			assert.Equal(t, tc.in.Type, decoded.Type)
		})
	}
}
