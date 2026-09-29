package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderMetadata_UnaryProjection(t *testing.T) {
	for _, tc := range []struct {
		name string
		part provider.GenerateContentPart
		want string
	}{
		{"openai text", provider.GenerateContentPart{Type: provider.ContentText, Text: "hi", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"msg_1","secret":"private-key"}`), "private": json.RawMessage(`{bad`)}}, `{"openai":{"itemId":"msg_1"}}`},
		{"anthropic call", provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "weather", Input: json.RawMessage(`{}`), ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"direct"},"token":"private-key"}`)}}, `{"anthropic":{"caller":{"type":"direct"}}}`},
		{"openai call", provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "weather", Input: json.RawMessage(`{}`), ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"fc_1"}`)}}, `{"openai":{"itemId":"fc_1"}}`},
		{"unknown only", provider.GenerateContentPart{Type: provider.ContentText, Text: "hi", ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{bad`)}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := validGenerateResult()
			result.Content = []provider.GenerateContentPart{tc.part}
			mapped, err := mapUnarySuccess(result, 1<<20)
			require.NoError(t, err)
			body, ok := encodeUnarySuccess(mapped, 1<<20)
			require.True(t, ok)
			var envelope struct {
				Content []map[string]json.RawMessage `json:"content"`
			}
			require.NoError(t, json.Unmarshal(body, &envelope))
			if tc.want == "" {
				assert.NotContains(t, envelope.Content[0], "providerMetadata")
			} else {
				assert.JSONEq(t, tc.want, string(envelope.Content[0]["providerMetadata"]))
			}
			assert.NotContains(t, string(body), "private-key")
		})
	}
}

func TestProviderMetadata_ValidationAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name, namespace, raw string
		limit                int64
		wantError            bool
	}{
		{"unknown malformed", "private", `{bad`, 1, false},
		{"unknown deeply nested", "private", `{"a":{"b":{"c":{"secret":"private-key"}}}}`, 1, false},
		{"recognized unknown sibling", "openai", `{"itemId":"msg_1","private":"private-key"}`, 100, false},
		{"recognized unknown large number", "openai", `{"itemId":"msg_1","unknown":1e999}`, 100, false},
		{"recognized paired surrogate", "openai", `{"itemId":"msg_1","unknown":"\ud83d\ude00"}`, 100, false},
		{"recognized escaped slash before surrogate", "openai", `{"itemId":"msg_1","unknown":"\\uD800"}`, 100, false},
		{"recognized unknown only", "openai", `{"private":"private-key"}`, 100, false},
		{"recognized at raw limit", "openai", `{"itemId":"msg_1"}`, int64(len(`{"itemId":"msg_1"}`)), false},
		{"recognized over raw limit", "openai", `{"itemId":"msg_1"}`, int64(len(`{"itemId":"msg_1"}`) - 1), true},
		{"recognized unknown only over limit", "openai", `{"private":"private-key"}`, 1, true},
		{"recognized malformed", "openai", `{bad`, 100, true},
		{"recognized duplicate", "openai", `{"itemId":"msg_1","itemId":"msg_1"}`, 100, true},
		{"recognized invalid UTF-8", "openai", string([]byte{0xff}), 100, true},
		{"recognized invalid surrogate in unknown field", "openai", `{"itemId":"msg_1","private":"\ud800"}`, 100, true},
		{"recognized invalid surrogate in key", "openai", `{"itemId":"msg_1","\udc00":"private"}`, 100, true},
		{"recognized invalid second surrogate", "openai", `{"itemId":"msg_1","unknown":"\ud83d\u0061"}`, 100, true},
		{"recognized deep unknown", "openai", `{"itemId":"msg_1","x":{"y":{"secret":"private-key"}}}`, 100, true},
		{"unsafe item ID", "openai", `{"itemId":"Bearer private-key"}`, 100, true},
		{"mismatched text ID", "openai", `{"itemId":"another"}`, 100, true},
		{"wrong caller", "anthropic", `{"caller":{"type":"tool"}}`, 100, true},
		{"extra caller", "anthropic", `{"caller":{"type":"direct","secret":"private-key"}}`, 100, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata, err := projectProviderMetadata(provider.ProviderMetadata{tc.namespace: json.RawMessage(tc.raw)}, true, "msg_1", tc.limit)
			if tc.wantError {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "private-key")
				return
			}
			require.NoError(t, err)
			if metadata != nil {
				body, marshalErr := json.Marshal(metadata)
				require.NoError(t, marshalErr)
				assert.NotContains(t, string(body), "private-key")
			}
		})
	}
}

func TestProviderMetadata_StreamProjection(t *testing.T) {
	h := newTestHandler(t, testLimits())
	state := &streamState{usedIDs: make(map[string]struct{}), tools: make(map[string]toolStreamState)}
	w := httptest.NewRecorder()
	for _, part := range []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "msg_1", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"msg_1","secret":"private-key"}`)}},
		{Type: provider.PartTextDelta, ID: "msg_1", Delta: "hi", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"bad"}`)}},
		{Type: provider.PartTextEnd, ID: "msg_1", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"msg_1"}`)}},
		{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "f", Input: `{}`, ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"direct"}}`)}},
		{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "f", Result: json.RawMessage(`{}`), ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"direct"}}`)}},
		{Type: provider.PartToolCall, ToolCallID: "another", ToolName: "f", Input: `{}`, ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"fc_1"}`)}},
		{Type: provider.PartToolResult, ToolCallID: "another", ToolName: "f", Result: json.RawMessage(`{}`), ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"fc_1"}`)}},
	} {
		assert.Equal(t, streamPartContinue, h.processStreamPart(w, state, part, "public-model"))
	}
	frames := strings.Split(strings.TrimSpace(w.Body.String()), "\n\n")
	require.Len(t, frames, 7)
	for _, index := range []int{0, 2} {
		assert.Contains(t, frames[index], `"providerMetadata":{"openai":{"itemId":"msg_1"}}`)
	}
	assert.NotContains(t, frames[1], "providerMetadata")
	for _, index := range []int{3, 4} {
		assert.Contains(t, frames[index], `"providerMetadata":{"anthropic":{"caller":{"type":"direct"}}}`)
	}
	for _, index := range []int{5, 6} {
		assert.Contains(t, frames[index], `"providerMetadata":{"openai":{"itemId":"fc_1"}}`)
	}
	assert.NotContains(t, w.Body.String(), "private-key")
}

func TestProviderMetadata_FailureBeforeCommitment(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"malformed", `{bad`},
		{"duplicate", `{"itemId":"msg_1","itemId":"msg_1"}`},
		{"unsafe identifier", `{"itemId":"Bearer private-key"}`},
		{"deep unknown sibling", `{"itemId":"msg_1","private":{"nested":{"token":"private-key"}}}`},
		{"invalid surrogate in unknown field", `{"itemId":"msg_1","private":"\ud800"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := validGenerateResult()
			result.Content[0].ProviderMetadata = provider.ProviderMetadata{"openai": json.RawMessage(tc.raw)}
			h := newTestHandler(t, testLimits())
			unary := httptest.NewRecorder()
			assert.False(t, h.writeUnarySuccess(unary, result))
			assert.Empty(t, unary.Body.String())

			state := newStreamState(8)
			stream := httptest.NewRecorder()
			part := provider.StreamPart{Type: provider.PartTextStart, ID: "msg_1", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(tc.raw)}}
			assert.Equal(t, streamPartAdapterFailure, h.processStreamPart(stream, state, part, "public"))
			assert.Empty(t, stream.Body.String())
			assert.Empty(t, state.activeID)
			assert.NotContains(t, stream.Body.String(), "private-key")
		})
	}
}

func TestProviderMetadata_RuntimeFailureIsTerminal(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	var modelCtx context.Context
	harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		modelCtx = ctx
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartTextStart, ID: "msg_1", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"msg_1"}`)}},
			provider.StreamPart{Type: provider.PartTextEnd, ID: "msg_1", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"Bearer private-key"}`)}},
			finishPart(),
		)}, nil
	}
	response := harness.serve(streamRequest(`{"prompt":[]}`))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"providerMetadata":{"openai":{"itemId":"msg_1"}}`)
	assert.Equal(t, 1, strings.Count(response.Body.String(), `"code":"internal_error"`))
	assert.NotContains(t, response.Body.String(), `"type":"text-end"`)
	assert.NotContains(t, response.Body.String(), `"type":"finish"`)
	assert.NotContains(t, response.Body.String(), "private-key")
	require.NotNil(t, modelCtx)
	assert.ErrorIs(t, modelCtx.Err(), context.Canceled)
}

func TestProviderMetadata_EncodedResponseLimits(t *testing.T) {
	result := validGenerateResult()
	result.Content[0].ProviderMetadata = provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"msg_1"}`)}
	mapped, err := mapUnarySuccess(result, 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	_, ok = encodeUnarySuccess(mapped, int64(len(body)))
	assert.True(t, ok)
	_, ok = encodeUnarySuccess(mapped, int64(len(body)-1))
	assert.False(t, ok)

	frame, ok := encodeStreamFrame(streamEvent{typeName: provider.PartTextStart, id: "msg_1", metadata: &projectedMetadata{OpenAI: &projectedOpenAI{ItemID: "msg_1"}}}, 1<<20)
	require.True(t, ok)
	_, ok = encodeStreamFrame(streamEvent{typeName: provider.PartTextStart, id: "msg_1", metadata: &projectedMetadata{OpenAI: &projectedOpenAI{ItemID: "msg_1"}}}, int64(len(frame)))
	assert.True(t, ok)
	_, ok = encodeStreamFrame(streamEvent{typeName: provider.PartTextStart, id: "msg_1", metadata: &projectedMetadata{OpenAI: &projectedOpenAI{ItemID: "msg_1"}}}, int64(len(frame)-1))
	assert.False(t, ok)
}
