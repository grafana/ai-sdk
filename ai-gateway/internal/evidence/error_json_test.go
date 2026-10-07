package evidence

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtectedErrorJSON_RawLexemes(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
		redacted           bool
	}{
		{"opaque keys and numbers", `{"\ud800":"\ud801","\u0064etail":[1e30,1.2300e+2,-0,"\u0061"]}`, `{"\ud800":"\ud801","\u0064etail":[1e30,1.2300e+2,-0,"\u0061"]}`, false},
		{"removed first and last", `{"apiKey":"secret","\ud800":{"signature":"secret","\u0064etail":1e30},"tenantId":"secret"}`, `{"\ud800":{"\u0064etail":1e30}}`, true},
		{"buffer refill", `{"large":"` + strings.Repeat("x", 80000) + `","nested":{"apiKey":"secret","tail":[{},[],null,false]}}`, `{"large":"` + strings.Repeat("x", 80000) + `","nested":{"tail":[{},[],null,false]}}`, true},
		{"html encoding", `{"detail":"<&>","escaped":"\u003c","apiKey":"secret"}`, `{"detail":"\u003c\u0026\u003e","escaped":"\u003c"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projected, err := protectedErrorJSON([]byte(tc.source), nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(projected.value))
			assert.Equal(t, tc.redacted, projected.redacted)
			assert.False(t, projected.echo)
		})
	}
}

func TestComponent_EncodedSize(t *testing.T) {
	for _, source := range []string{`{"apiKey":"secret","detail":"<&>"}`, `"\ud800"`, `{"detail":1.2300e+2}`} {
		projected, invalid := projectError(provider.NewAPICallError(provider.APICallErrorOptions{Data: json.RawMessage(source)}))
		require.False(t, invalid)
		require.Equal(t, Available, projected.Details.State)
		require.Positive(t, projected.Details.encodedBytes)
		raw, err := json.Marshal(projected.Details)
		require.NoError(t, err)
		size, err := projected.Details.encodedSize()
		require.NoError(t, err)
		assert.Equal(t, len(raw), size)
	}
	for _, component := range []*Component{{State: OverLimit, Reason: AggregateLimit}, {State: Available, Value: json.RawMessage("{")}} {
		raw, marshalErr := json.Marshal(component)
		size, sizeErr := component.encodedSize()
		assert.Equal(t, len(raw), size)
		assert.Equal(t, marshalErr, sizeErr)
	}
}
