package bedrock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReasoningNativeHTTPHighLevelReplay(t *testing.T) {
	requests := make(chan map[string]json.RawMessage, 2)
	frames := encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"reasoningContent":{"redactedContent":"opaque-"}}}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"reasoningContent":{"redactedContent":"final"}}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"messageStop":{"stopReason":"end_turn"}}`,
		`{"metadata":{"usage":{"inputTokens":1,"outputTokens":2,"totalTokens":3}}}`,
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		requests <- body
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		_, _ = w.Write(frames)
	}))
	defer server.Close()
	model := New(testAnthropicModel, WithBaseURL(server.URL), WithHTTPClient(server.Client()), WithBearerToken("dummy"), WithRegion("us-east-1"))
	first, err := aisdk.GenerateText(context.Background(), model, aisdk.WithModelMessages(provider.UserText("think")))
	require.NoError(t, err)
	<-requests
	_, err = aisdk.GenerateText(context.Background(), model, aisdk.WithModelMessages(first.Response.Messages...))
	require.NoError(t, err)
	body := <-requests
	var messages []struct {
		Content []json.RawMessage `json:"content"`
	}
	require.NoError(t, json.Unmarshal(body["messages"], &messages))
	require.Len(t, messages[0].Content, 1)
	assert.JSONEq(t, `{"reasoningContent":{"redactedContent":"opaque-final"}}`, string(messages[0].Content[0]))
}

// Synthetic native events exercise the frozen upstream redactedContent path;
// these are not recorded conformance fixtures.
func TestReasoningRedactedContentContinuation(t *testing.T) {
	parts := drainStream(t, encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"reasoningContent":{"redactedContent":"opaque-"}}}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"reasoningContent":{"redactedContent":"continuation"}}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"messageStop":{"stopReason":"end_turn"}}`,
		`{"metadata":{"usage":{"inputTokens":1,"outputTokens":2,"totalTokens":3}}}`,
	), requestMeta{})
	var end *provider.StreamPart
	for i := range parts {
		if parts[i].Type == provider.PartReasoningEnd {
			end = &parts[i]
		}
		assert.NotEqual(t, provider.PartReasoningDelta, parts[i].Type)
	}
	require.NotNil(t, end)
	for _, namespace := range []string{"amazonBedrock", "bedrock"} {
		assert.JSONEq(t, `{"redactedContent":"opaque-continuation"}`, string(end.ProviderMetadata[namespace]))
		reasoning := provider.ReasoningPart("")
		reasoning.ProviderOptions = provider.ProviderOptions{namespace: provider.RawProviderOption{Key: namespace, Raw: end.ProviderMetadata[namespace]}}
		req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(reasoning)}})
		require.Len(t, req.Messages[0].Content, 1)
		raw, err := json.Marshal(req.Messages[0].Content[0].ReasoningContent)
		require.NoError(t, err)
		assert.JSONEq(t, `{"redactedContent":"opaque-continuation"}`, string(raw))
	}
}

func TestReasoningReplayPreservesPresentEmptyValues(t *testing.T) {
	for _, tc := range []struct{ metadata, native string }{
		{`{"signature":""}`, `{"reasoningText":{"text":" signed  ","signature":""}}`},
		{`{"redactedContent":""}`, `{"redactedContent":""}`},
		{`{"redactedData":""}`, `{"redactedReasoning":{"data":""}}`},
	} {
		reasoning := provider.ReasoningPart(" signed  ")
		reasoning.ProviderOptions = provider.ProviderOptions{"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(tc.metadata)}}
		req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(reasoning)}})
		raw, err := json.Marshal(req.Messages[0].Content[0].ReasoningContent)
		require.NoError(t, err)
		assert.JSONEq(t, tc.native, string(raw))
	}
}
