package byok

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/routing"
	"github.com/grafana/ai-sdk/provider"
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

// newAll returns the model that tries every account of request in order.
func newAll(request Request, client *http.Client) (provider.LanguageModel, error) {
	var placements []routing.Placement
	for g, destination := range request.Destinations {
		for c := range destination.Accounts {
			placements = append(placements, routing.Placement{Group: g, Candidate: c})
		}
	}
	return New(request, placements, client)
}

func TestDecodeRequest_FallbackModels(t *testing.T) {
	const accounts = `"byok":{"anthropic":[{"apiKey":"a1"},{"apiKey":"a2"}],"openai":[{"apiKey":"o1"}]}`
	got, err := decodeGateway("anthropic/x", json.RawMessage(`{`+accounts+`,"models":["openai/y","anthropic/z/w"]}`), nil)
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
