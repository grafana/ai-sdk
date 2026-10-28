package byok

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/routing"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeGateway decodes raw call-level gateway controls as a BYOK caller sends
// them.
func decodeGateway(selector string, raw json.RawMessage, approvals map[Provider][]string) (Request, error) {
	controls, err := routing.Decode(raw, routing.RequestAccountAccess)
	if err != nil {
		return Request{}, err
	}
	return DecodeRequest(selector, controls, approvals)
}

func TestDecodeRequest_FallbackModels(t *testing.T) {
	const accounts = `"byok":{"anthropic":[{"apiKey":"a1"},{"apiKey":"a2"}],"openai":[{"apiKey":"o1"}]}`
	got, err := decodeGateway("anthropic/x", json.RawMessage(`{`+accounts+`,"models":["openai/y","anthropic/z/w"],"providerTimeouts":{"byok":{"anthropic":2000}}}`), nil)
	require.NoError(t, err)
	require.Len(t, got.Destinations, 3)
	for i, want := range []struct {
		provider Provider
		model    string
		keys     []string
	}{
		{Anthropic, "x", []string{"a1", "a2"}},
		{OpenAI, "y", []string{"o1"}},
		{Anthropic, "z/w", []string{"a1", "a2"}},
	} {
		destination := got.Destinations[i]
		assert.Equal(t, want.provider, destination.Provider)
		assert.Equal(t, want.model, destination.Model)
		keys := make([]string, len(destination.Accounts))
		for j, account := range destination.Accounts {
			keys[j] = account.APIKey
		}
		assert.Equal(t, want.keys, keys)
	}
	assert.Equal(t, map[Provider]time.Duration{Anthropic: 2 * time.Second}, got.Timeouts)
}

func TestDecodeRequest_RoutingRefusals(t *testing.T) {
	const accounts = `"byok":{"anthropic":[{"apiKey":"dummy-secret"}]}`
	for _, tc := range []struct {
		name, raw string
		want      error
	}{
		{name: "fallback provider without accounts", raw: `{` + accounts + `,"models":["openai/y"]}`, want: ErrInvalidRequest},
		{name: "fallback without split", raw: `{` + accounts + `,"models":["configured-alias"]}`, want: ErrInvalidSelector},
		{name: "fallback with unsupported provider", raw: `{` + accounts + `,"models":["bedrock/y"]}`, want: ErrInvalidSelector},
		{name: "fallback with empty model", raw: `{` + accounts + `,"models":["anthropic/"]}`, want: ErrInvalidSelector},
		{name: "timeout for unsupported provider", raw: `{` + accounts + `,"providerTimeouts":{"byok":{"bedrock":1000}}}`, want: execution.ErrProviderTimeouts},
		{name: "timeout provider case variant", raw: `{` + accounts + `,"providerTimeouts":{"byok":{"Anthropic":1000}}}`, want: execution.ErrProviderTimeouts},
		{name: "requested credentials precede fallback", raw: `{"byok":{"openai":[{"apiKey":"o"}]},"models":["bedrock/y"]}`, want: ErrInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeGateway("anthropic/x", json.RawMessage(tc.raw), nil)
			require.ErrorIs(t, err, tc.want)
			if tc.want == ErrInvalidRequest {
				assert.NotErrorIs(t, err, ErrInvalidSelector)
			}
			assert.Zero(t, got)
			assert.NotContains(t, err.Error(), "dummy-secret")
		})
	}
}
