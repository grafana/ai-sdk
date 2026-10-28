package routing

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecode_Controls(t *testing.T) {
	empty := []string{}
	anthropic := []string{"anthropic"}
	for _, tc := range []struct {
		name   string
		raw    string
		access Access
		want   Controls
	}{
		{name: "absent", access: ConfiguredAccess},
		{name: "null", raw: `null`, access: ConfiguredAccess},
		{name: "empty object", raw: `{}`, access: ConfiguredAccess},
		{name: "routing controls", raw: `{"models":["b","c"],"order":["openai"],"only":["anthropic"]}`, access: ConfiguredAccess, want: Controls{Models: []string{"b", "c"}, Order: []string{"openai"}, Only: &anthropic}},
		{name: "null and empty lists are absent", raw: `{"models":null,"order":[],"only":null}`, access: ConfiguredAccess},
		{name: "empty only is kept", raw: `{"only":[]}`, access: ConfiguredAccess, want: Controls{Only: &empty}},
		{name: "repeated values are kept", raw: `{"order":["openai","openai"]}`, access: ConfiguredAccess, want: Controls{Order: []string{"openai", "openai"}}},
		{name: "values are not folded", raw: `{"only":[" Anthropic "]}`, access: ConfiguredAccess, want: Controls{Only: &[]string{" Anthropic "}}},
		{name: "last duplicate member wins", raw: `{"order":["a"],"order":["b"]}`, access: ConfiguredAccess, want: Controls{Order: []string{"b"}}},
		{name: "request accounts", raw: `{"byok":{"openai":[{"apiKey":"k"}]},"models":["anthropic/x"],"providerTimeouts":{"byok":{"anthropic":1,"openai":600000}}}`, access: RequestAccountAccess, want: Controls{
			Models: []string{"anthropic/x"}, BYOK: json.RawMessage(`{"openai":[{"apiKey":"k"}]}`), ProviderTimeouts: map[string]int{"anthropic": 1, "openai": 600000},
		}},
		{name: "null provider timeouts", raw: `{"providerTimeouts":null}`, access: RequestAccountAccess},
		{name: "null byok timeouts", raw: `{"providerTimeouts":{"byok":null}}`, access: RequestAccountAccess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw json.RawMessage
			if tc.raw != "" {
				raw = json.RawMessage(tc.raw)
			}
			got, err := Decode(raw, tc.access)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecode_Refusals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		raw    string
		access Access
		want   error
	}{
		{name: "configured byok", raw: `{"byok":{"openai":[{"apiKey":"dummy-secret"}]}}`, access: ConfiguredAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "configured provider timeouts", raw: `{"providerTimeouts":{"byok":{"openai":1}}}`, access: ConfiguredAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "case variant", raw: `{"Models":["b"]}`, access: ConfiguredAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "byok case variant", raw: `{"BYOK":{}}`, access: RequestAccountAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "sort", raw: `{"sort":"cost"}`, access: RequestAccountAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "has", raw: `{"has":["vision"]}`, access: ConfiguredAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "zero data retention", raw: `{"zeroDataRetention":true}`, access: ConfiguredAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "user", raw: `{"user":"u"}`, access: RequestAccountAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "provider timeouts member", raw: `{"providerTimeouts":{"system":{"openai":1}}}`, access: RequestAccountAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "unsupported control precedes malformed list", raw: `{"order":42,"sort":"cost"}`, access: ConfiguredAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "timeout member precedes malformed list", raw: `{"order":42,"providerTimeouts":{"system":{}}}`, access: RequestAccountAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "not an object", raw: `[]`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "models not an array", raw: `{"models":"b"}`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "conditional models entry", raw: `{"models":[{"model":"b","when":{"confidenceBelow":0.5}}]}`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "empty string", raw: `{"order":[""]}`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "null entry", raw: `{"only":[null]}`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "number entry", raw: `{"only":[1]}`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "malformed list precedes timeouts", raw: `{"order":42,"providerTimeouts":{"byok":{"openai":0}}}`, access: RequestAccountAccess, want: execution.ErrRoutingControls},
		{name: "timeouts not an object", raw: `{"providerTimeouts":[]}`, access: RequestAccountAccess, want: execution.ErrProviderTimeouts},
		{name: "zero timeout", raw: `{"providerTimeouts":{"byok":{"openai":0}}}`, access: RequestAccountAccess, want: execution.ErrProviderTimeouts},
		{name: "timeout above bound", raw: `{"providerTimeouts":{"byok":{"openai":600001}}}`, access: RequestAccountAccess, want: execution.ErrProviderTimeouts},
		{name: "fractional timeout", raw: `{"providerTimeouts":{"byok":{"openai":1.5}}}`, access: RequestAccountAccess, want: execution.ErrProviderTimeouts},
		{name: "exponent timeout", raw: `{"providerTimeouts":{"byok":{"openai":1e3}}}`, access: RequestAccountAccess, want: execution.ErrProviderTimeouts},
		{name: "string timeout", raw: `{"providerTimeouts":{"byok":{"openai":"1000"}}}`, access: RequestAccountAccess, want: execution.ErrProviderTimeouts},
		{name: "null timeout", raw: `{"providerTimeouts":{"byok":{"openai":null}}}`, access: RequestAccountAccess, want: execution.ErrProviderTimeouts},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decode(json.RawMessage(tc.raw), tc.access)
			require.ErrorIs(t, err, tc.want)
			assert.Zero(t, got)
			assert.NotContains(t, err.Error(), "dummy-secret")
		})
	}
}
