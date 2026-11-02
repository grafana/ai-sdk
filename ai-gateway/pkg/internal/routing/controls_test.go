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
		{name: "null and empty lists do not route", raw: `{"models":null,"order":[],"only":null}`, access: ConfiguredAccess, want: Controls{Order: []string{}}},
		{name: "empty only is kept", raw: `{"only":[]}`, access: ConfiguredAccess, want: Controls{Only: &empty}},
		{name: "repeated values are kept", raw: `{"order":["openai","openai"]}`, access: ConfiguredAccess, want: Controls{Order: []string{"openai", "openai"}}},
		{name: "values are not folded", raw: `{"only":[" Anthropic "]}`, access: ConfiguredAccess, want: Controls{Only: &[]string{" Anthropic "}}},
		{name: "last duplicate member wins", raw: `{"order":["a"],"order":["b"]}`, access: ConfiguredAccess, want: Controls{Order: []string{"b"}}},
		{name: "request accounts", raw: `{"byok":{"openai":[{"apiKey":"k"}]},"models":["anthropic/x"]}`, access: RequestAccountAccess, want: Controls{
			Models: []string{"anthropic/x"}, BYOK: json.RawMessage(`{"openai":[{"apiKey":"k"}]}`),
		}},
		{name: "case variants match", raw: `{"Models":["b"],"ONLY":["openai"]}`, access: ConfiguredAccess, want: Controls{Models: []string{"b"}, Only: &[]string{"openai"}}},
		{name: "byok case variant", raw: `{"BYOK":{}}`, access: RequestAccountAccess, want: Controls{BYOK: json.RawMessage(`{}`)}},
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
		{name: "configured byok case variant", raw: `{"Byok":{}}`, access: ConfiguredAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "provider timeouts", raw: `{"providerTimeouts":{"byok":{"openai":1}}}`, access: RequestAccountAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "sort", raw: `{"sort":"cost"}`, access: RequestAccountAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "has", raw: `{"has":["vision"]}`, access: ConfiguredAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "zero data retention", raw: `{"zeroDataRetention":true}`, access: ConfiguredAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "user", raw: `{"user":"u"}`, access: RequestAccountAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "unsupported control precedes malformed list", raw: `{"order":42,"sort":"cost"}`, access: ConfiguredAccess, want: execution.ErrUnsupportedGatewayControl},
		{name: "not an object", raw: `[]`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "models not an array", raw: `{"models":"b"}`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "conditional models entry", raw: `{"models":[{"model":"b","when":{"confidenceBelow":0.5}}]}`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "empty string", raw: `{"order":[""]}`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "null entry", raw: `{"only":[null]}`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "number entry", raw: `{"only":[1]}`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
		{name: "case variant entry", raw: `{"MODELS":[""]}`, access: ConfiguredAccess, want: execution.ErrRoutingControls},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decode(json.RawMessage(tc.raw), tc.access)
			require.ErrorIs(t, err, tc.want)
			assert.Zero(t, got)
			assert.NotContains(t, err.Error(), "dummy-secret")
		})
	}
}
