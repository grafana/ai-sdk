package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectError_DetailDispositions(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		state        DispositionState
		reason       Reason
		redacted     bool
	}{
		{"empty source", "", Unavailable, ProducerDoesNotExpose, false},
		{"null", "null", Available, "", false},
		{"opaque surrogate", `{"detail":"\ud800"}`, Available, "", false},
		{"redacted surrogate", `{"apiKey":"secret","detail":{"items":["\ud800",{"\ud800":1e30,"signature":"secret"}]}}`, Available, "", true},
		{"empty object", "{}", Available, "", false},
		{"empty array", "[]", Available, "", false},
		{"empty string", `""`, Available, "", false},
		{"false", "false", Available, "", false},
		{"zero", "0", Available, "", false},
		{"invalid", "{", Malformed, InvalidJSON, false},
		{"duplicate credential subtree", `{"error":{"apiKey":"secret"},"error":{"message":"safe"}}`, Malformed, InvalidJSON, false},
		{"nested duplicate", `{"detail":{"message":"secret","message":"safe"}}`, Malformed, InvalidJSON, false},
		{"invalid utf8", string([]byte{'"', 255, '"'}), Malformed, InvalidJSON, false},
		{"source exact", `"` + strings.Repeat("x", SourceBytes-2) + `"`, OverLimit, EncodedLimit, false},
		{"source over", strings.Repeat("x", SourceBytes+1), OverLimit, SourceLimit, false},
		{"encoded exact", `"` + strings.Repeat("x", ComponentBytes-32) + `"`, Available, "", false},
		{"encoded over", `"` + strings.Repeat("x", ComponentBytes-31) + `"`, OverLimit, EncodedLimit, false},
		{"escaping expansion", `"` + strings.Repeat("<", ComponentBytes/6+1) + `"`, OverLimit, EncodedLimit, false},
		{"credential only", `{"authorization":"secret"}`, Redacted, CredentialSource, false},
		{"partial nested", `{"error":{"message":"safe","detail":{"model":"sk-ordinary","token":"ordinary"},"request":{"headers":{"apiKey":"secret"}},"tenantId":"private"}}`, Available, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(tc.source)
			failure := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 429, Message: "safe summary", Data: raw})
			projected, invalid := projectError(failure)
			require.False(t, invalid)
			require.NotNil(t, projected)
			assert.Equal(t, 429, projected.StatusCode)
			assert.Equal(t, tc.state, projected.Details.State)
			assert.Equal(t, tc.reason, projected.Details.Reason)
			assert.Equal(t, tc.redacted, projected.Details.Redacted)
			assert.Equal(t, raw, failure.Data)
			if tc.name == "encoded exact" {
				encoded, err := json.Marshal(projected.Details)
				require.NoError(t, err)
				assert.Len(t, encoded, ComponentBytes)
			}
			if tc.name == "opaque surrogate" || tc.name == "redacted surrogate" {
				assert.Contains(t, string(projected.Details.Value), `\ud800`)
				assert.NotContains(t, string(projected.Details.Value), "secret")
				assert.NotContains(t, string(projected.Details.Value), "\uFFFD")
			}
			if tc.name == "partial nested" {
				assert.Contains(t, string(projected.Details.Value), "sk-ordinary")
				assert.Contains(t, string(projected.Details.Value), `"token":"ordinary"`)
				assert.NotContains(t, string(projected.Details.Value), "secret")
				assert.NotContains(t, string(projected.Details.Value), "private")
			}
		})
	}
}

func TestProjectError_KnownCredentialEcho(t *testing.T) {
	for _, source := range []string{"", `{"error":{"message":"echo dummy-api-key", "detail":"ordinary"}}`} {
		native := provider.NewAPICallError(provider.APICallErrorOptions{Message: "echo dummy-api-key", Data: json.RawMessage(source)})
		projected, invalid := projectError(native, "dummy-api-key")
		require.False(t, invalid)
		assert.Empty(t, projected.Message)
		encoded, err := json.Marshal(projected)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "dummy-api-key")
		if source != "" {
			assert.Equal(t, Redacted, projected.Details.State)
		}
	}
}

func TestProjectError_SummaryAndAttribution(t *testing.T) {
	native := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, URL: "https://private.invalid", Message: "formatted secret", Data: json.RawMessage(`{"error":{"message":"native summary","type":"native","code":429,"apiKey":"secret"}}`)})
	projected, invalid := projectError(fmt.Errorf("candidate wrapper: %w", native))
	require.False(t, invalid)
	assert.Equal(t, "native summary", projected.Message)
	assert.Equal(t, "native", projected.Type)
	assert.JSONEq(t, "429", string(projected.Code))
	assert.NotContains(t, string(projected.Details.Value), "secret")
	projected, invalid = projectError(errors.Join(errors.New("unknown"), native))
	assert.Nil(t, projected)
	assert.False(t, invalid)
	projected, invalid = projectError(provider.NewAPICallError(provider.APICallErrorOptions{Message: strings.Repeat("x", EssentialBytes+1)}))
	assert.Nil(t, projected)
	assert.True(t, invalid)
}
