package v4

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Synthetic native HTTP witness, not a recorded provider fixture. The separate
// client/core witness proves assembly; this proves both HTTP mappings and the
// resulting native signature-bearing request.
func TestReasoningNativeAnthropicTwoRequests(t *testing.T) {
	requests := make(chan map[string]any, 2)
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"response","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"thinking","thinking":" signed  ","signature":"opaque-signature"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer native.Close()
	model := anthropicprovider.New("test-key", "claude-sonnet-4-6", anthropicprovider.WithRequestOptions(option.WithBaseURL(native.URL), option.WithHTTPClient(native.Client()), option.WithMaxRetries(0)))
	harness := newRuntimeHarness(t, testLimits())
	harness.model.generate = func(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
		return model.DoGenerate(ctx, opts)
	}
	first := harness.serve(validRequest(`{"prompt":[{"role":"user","content":[{"type":"text","text":"think"}]}],"maxOutputTokens":64}`))
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	<-requests
	var output struct {
		Content []struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			Metadata json.RawMessage `json:"providerMetadata"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &output))
	require.Len(t, output.Content, 1)
	assert.Equal(t, "reasoning", output.Content[0].Type)
	text, err := json.Marshal(output.Content[0].Text)
	require.NoError(t, err)
	second := harness.serve(validRequest(`{"prompt":[{"role":"assistant","content":[{"type":"reasoning","text":` + string(text) + `,"providerOptions":` + string(output.Content[0].Metadata) + `}]},{"role":"user","content":[{"type":"text","text":"continue"}]}],"maxOutputTokens":64}`))
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	body := <-requests
	part := body["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	assert.Equal(t, map[string]any{"type": "thinking", "thinking": " signed  ", "signature": "opaque-signature"}, part)
}
