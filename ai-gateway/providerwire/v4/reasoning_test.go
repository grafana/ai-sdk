package v4

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReasoningRequest(t *testing.T) {
	body := []byte(`{"prompt":[{"role":"assistant","content":[{"type":"reasoning","text":"","providerOptions":{"anthropic":{"signature":"signed"}}},{"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":""}}]}]}`)
	opts, failure := mapWireRequest(body)
	require.Nil(t, failure)
	require.Len(t, opts.Prompt[0].Content, 2)
	assert.Equal(t, provider.ContentPartTypeReasoning, opts.Prompt[0].Content[0].Type)
	require.NotNil(t, opts.Prompt[0].Content[0].ProviderOptions["anthropic"])
	assert.True(t, opts.Prompt[0].Content[1].Data.IsData())
}

func TestReasoningUnary(t *testing.T) {
	data := provider.Base64DataContent("")
	result := validGenerateResult()
	result.Content = []provider.GenerateContentPart{
		{Type: provider.ContentReasoning, Text: "", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"r1","reasoningEncryptedContent":null,"backend":"secret"}`)}},
		{Type: provider.ContentReasoningFile, MediaType: "image/png", Data: &data},
	}
	mapped, err := mapUnarySuccess(result, 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	assert.Contains(t, string(body), `"text":""`)
	assert.Contains(t, string(body), `"reasoningEncryptedContent":null`)
	assert.Contains(t, string(body), `"data":{"type":"data","data":""}`)
	assert.NotContains(t, string(body), "secret")
	compiled, err := schema.CompileSchema(unarySuccessSchemaJSON)
	require.NoError(t, err)
	require.NoError(t, compiled.Validate(body))
}

func TestReasoningConcurrentLifecycle(t *testing.T) {
	h := &handler{limits: Limits{StreamFrameBytes: 1 << 20}}
	state := newStreamState(100)
	w := httptest.NewRecorder()
	parts := []provider.StreamPart{
		{Type: provider.PartReasoningStart, ID: "1"},
		{Type: provider.PartReasoningStart, ID: "2"},
		{Type: provider.PartTextStart, ID: "1"},
		{Type: provider.PartReasoningDelta, ID: "1", Delta: ""},
		{Type: provider.PartReasoningFile, MediaType: "image/png", Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData}},
		{Type: provider.PartReasoningDelta, ID: "2", Delta: "thought"},
		{Type: provider.PartReasoningEnd, ID: "1", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"end-only"}`)}},
		{Type: provider.PartReasoningEnd, ID: "2", ProviderMetadata: provider.ProviderMetadata{}},
		{Type: provider.PartTextEnd, ID: "1"},
	}
	for _, part := range parts {
		require.Equal(t, streamPartContinue, h.processStreamPart(w, state, part, "public/model"), part.Type)
	}
	require.Equal(t, streamPartFinished, h.processStreamPart(w, state, finishPart(), "public/model"))
	assert.Contains(t, w.Body.String(), `"delta":""`)
	assert.Contains(t, w.Body.String(), `"providerMetadata":{}`)
	assert.Contains(t, w.Body.String(), "end-only")
	assert.Equal(t, len(parts)+1, strings.Count(w.Body.String(), "data: "))
	requireStreamBodyMatchesSchema(t, w.Body.String())
}

func TestReasoningRequestRejectsBeforeResolution(t *testing.T) {
	for _, content := range []string{
		`{"type":"reasoning"}`, `{"type":"reasoning","text":null}`,
		`{"type":"reasoning-file","mediaType":"image/png","data":null}`,
		`{"type":"reasoning-file","mediaType":"image/png","data":{"type":"data"}}`,
		`{"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":"","url":"https://example.test"}}`,
		`{"type":"reasoning-file","mediaType":"image/png","data":{"type":"text","text":""}}`,
		`{"type":"reasoning-file","mediaType":"image/png","data":{"type":"reference","reference":{}}}`,
		`{"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":""},"filename":"private"}`,
		`{"type":"reasoning","text":"","providerOptions":{"anthropic":null}}`,
		`{"type":"reasoning","text":"","providerOptions":{"gateway":{"token":"private"}}}`,
	} {
		for _, streaming := range []string{"true", "false"} {
			harness := newRuntimeHarness(t, testLimits())
			req := validRequest(`{"prompt":[{"role":"assistant","content":[` + content + `]}]}`)
			req.Header.Set(HeaderStreaming, streaming)
			response := harness.serve(req)
			require.Equal(t, http.StatusBadRequest, response.Code, content)
			assert.Zero(t, harness.resolver.callCount())
			assert.Zero(t, harness.model.callCount())
			assert.NotContains(t, response.Body.String(), "private")
		}
	}
	for _, role := range []string{"user", "tool", "system"} {
		harness := newRuntimeHarness(t, testLimits())
		response := harness.serve(validRequest(`{"prompt":[{"role":"` + role + `","content":[{"type":"reasoning","text":"private"}]}]}`))
		require.Equal(t, http.StatusBadRequest, response.Code)
		assert.Zero(t, harness.resolver.callCount())
	}
}

func TestReasoningMetadataProjection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   provider.ProviderMetadata
		want    string
		invalid bool
	}{
		{name: "empty", input: provider.ProviderMetadata{}, want: `{}`},
		{name: "unknown", input: provider.ProviderMetadata{"backend": json.RawMessage(`{"credentials":"private"}`)}, want: `{}`},
		{name: "empty namespace", input: provider.ProviderMetadata{"anthropic": json.RawMessage(`{}`)}, want: `{"anthropic":{}}`},
		{name: "redacted", input: provider.ProviderMetadata{"bedrock": json.RawMessage(`{"signature":"","redactedData":"redacted","redactedContent":"opaque","headers":{"secret":"private"}}`)}, want: `{"bedrock":{"signature":"","redactedData":"redacted","redactedContent":"opaque"}}`},
		{name: "null encrypted", input: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"r1","reasoningEncryptedContent":null}`)}, want: `{"openai":{"itemId":"r1","reasoningEncryptedContent":null}}`},
		{name: "bad signature", input: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":false}`)}, invalid: true},
		{name: "null signature", input: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":null}`)}, invalid: true},
		{name: "null namespace", input: provider.ProviderMetadata{"openai": json.RawMessage(`null`)}, invalid: true},
		{name: "invalid utf8", input: provider.ProviderMetadata{"openai": json.RawMessage{'"', 255, '"'}}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapped, err := projectReasoningMetadata(tc.input, 1024)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			encoded, err := json.Marshal(mapped)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
	mapped, err := projectReasoningMetadata(nil, 1024)
	require.NoError(t, err)
	assert.Nil(t, mapped)
	_, err = projectReasoningMetadata(provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"` + strings.Repeat("x", 1024) + `"}`)}, 1024)
	require.Error(t, err)
}

func TestReasoningFrameBoundsAndLifecycle(t *testing.T) {
	for _, part := range []provider.StreamPart{
		{Type: provider.PartReasoningDelta, ID: "r", Delta: "\"<>&☃", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"signed"}`)}},
		{Type: provider.PartReasoningFile, MediaType: "image/png", Data: &provider.StreamFileData{Bytes: []byte{1, 2, 3}}},
	} {
		event := streamEvent{typeName: part.Type, id: part.ID, delta: part.Delta, reasoningMetadata: part.ProviderMetadata, mediaType: part.MediaType, fileData: part.Data}
		frame, ok := encodeStreamFrame(event, 1<<20)
		require.True(t, ok)
		for _, offset := range []int64{-1, 0, 1} {
			got, ok := encodeStreamFrame(event, int64(len(frame))+offset)
			assert.Equal(t, offset >= 0, ok)
			assert.LessOrEqual(t, int64(len(got)), int64(len(frame))+offset)
		}
	}
	for _, parts := range [][]provider.StreamPart{
		{{Type: provider.PartReasoningDelta, ID: "r"}},
		{{Type: provider.PartReasoningEnd, ID: "r"}},
		{{Type: provider.PartReasoningStart, ID: "r"}, {Type: provider.PartReasoningStart, ID: "r"}},
		{{Type: provider.PartReasoningStart, ID: "r"}, finishPart()},
		{{Type: provider.PartReasoningStart, ID: "r"}, {Type: provider.PartResponseMeta}},
		{{Type: provider.PartReasoningStart, ID: "r"}, {Type: provider.PartReasoningEnd, ID: "r"}, {Type: provider.PartReasoningStart, ID: "r"}},
	} {
		h := &handler{limits: Limits{StreamFrameBytes: 1024}}
		state := newStreamState(100)
		w := httptest.NewRecorder()
		for i, part := range parts {
			want := streamPartContinue
			if i == len(parts)-1 {
				want = streamPartAdapterFailure
			}
			assert.Equal(t, want, h.processStreamPart(w, state, part, "public/model"))
		}
	}
}
