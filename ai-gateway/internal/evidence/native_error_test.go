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
		{"duplicate credential subtree", `{"error":{"apiKey":"secret"},"error":{"message":"safe"}}`, Available, "", false},
		{"nested duplicate", `{"detail":{"message":"secret","message":"safe"}}`, Available, "", false},
		{"invalid utf8", string([]byte{'"', 255, '"'}), Malformed, InvalidJSON, false},
		{"source exact", `"` + strings.Repeat("x", maxSourceBytes-2) + `"`, OverLimit, EncodedLimit, false},
		{"source over", strings.Repeat("x", maxSourceBytes+1), OverLimit, SourceLimit, false},
		{"encoded exact", `"` + strings.Repeat("x", maxComponentBytes-32) + `"`, Available, "", false},
		{"encoded over", `"` + strings.Repeat("x", maxComponentBytes-31) + `"`, OverLimit, EncodedLimit, false},
		{"escaping expansion", `"` + strings.Repeat("<", maxComponentBytes/6+1) + `"`, OverLimit, EncodedLimit, false},
		{"credential only", `{"authorization":"secret"}`, Redacted, CredentialSource, false},
		{"partial nested", `{"error":{"message":"safe","detail":{"model":"sk-ordinary","token":"ordinary"},"request":{"headers":{"apiKey":"secret"}},"tenantId":"private"}}`, Available, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(tc.source)
			failure := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 429, Message: "safe summary", Data: raw})
			projected, err := normalizeError(failure, nil)
			require.NoError(t, err)
			require.NotNil(t, projected)
			assert.Equal(t, 429, projected.StatusCode)
			assert.Equal(t, tc.state, readDetails(t, projected).State)
			assert.Equal(t, tc.reason, readDetails(t, projected).Reason)
			assert.Equal(t, tc.redacted, readDetails(t, projected).Redacted)
			assert.Equal(t, raw, failure.Data)
			if tc.name == "encoded exact" {
				encoded, err := json.Marshal(projected.Details)
				require.NoError(t, err)
				assert.Len(t, encoded, maxComponentBytes)
			}
			if tc.name == "opaque surrogate" || tc.name == "redacted surrogate" {
				assert.Contains(t, string(readDetails(t, projected).Value), "\uFFFD")
				assert.NotContains(t, string(readDetails(t, projected).Value), "secret")
				assert.NotContains(t, string(readDetails(t, projected).Value), `\ud800`)
			}
			if tc.name == "partial nested" {
				assert.Contains(t, string(readDetails(t, projected).Value), "sk-ordinary")
				assert.Contains(t, string(readDetails(t, projected).Value), `"token":"ordinary"`)
				assert.NotContains(t, string(readDetails(t, projected).Value), "secret")
				assert.NotContains(t, string(readDetails(t, projected).Value), "private")
			}
		})
	}
}

func TestProjectError_KnownCredentialEcho(t *testing.T) {
	for _, source := range []string{"", `{"error":{"message":"echo dummy-api-key", "detail":"ordinary"}}`} {
		native := provider.NewAPICallError(provider.APICallErrorOptions{Message: "echo dummy-api-key", Data: json.RawMessage(source)})
		projected, err := normalizeError(native, []string{"dummy-api-key"})
		require.NoError(t, err)
		assert.Empty(t, projected.Message)
		encoded, err := json.Marshal(projected)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "dummy-api-key")
		if source != "" {
			assert.Equal(t, Redacted, readDetails(t, projected).State)
		}
	}
}

func TestProjectError_SummaryAndAttribution(t *testing.T) {
	native := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, URL: "https://private.invalid", Message: "formatted secret", Data: json.RawMessage(`{"error":{"message":"native summary","type":"native","code":429,"apiKey":"secret"}}`)})
	projected, err := normalizeError(fmt.Errorf("candidate wrapper: %w", native), nil)
	require.NoError(t, err)
	assert.Equal(t, "native summary", projected.Message)
	assert.Equal(t, "native", projected.Type)
	assert.JSONEq(t, "429", string(projected.Code))
	assert.NotContains(t, string(readDetails(t, projected).Value), "secret")
	projected, err = normalizeError(errors.Join(errors.New("unknown"), native), nil)
	assert.Nil(t, projected)
	assert.NoError(t, err)
	projected, err = normalizeError(provider.NewAPICallError(provider.APICallErrorOptions{Message: strings.Repeat("x", maxEssentialBytes+1)}), nil)
	assert.Nil(t, projected)
	assert.Error(t, err)
}

func TestNormalizeError_StandardJSON(t *testing.T) {
	for _, tc := range []struct {
		name, source, message, want string
		state                       DispositionState
	}{
		{"duplicate discarded echo", `{"message":"dummy-api-key","message":"safe"}`, "safe", `{"message":"safe"}`, Available},
		{"duplicate surviving echo", `{"message":"safe","message":"dummy-api-key"}`, "", "", Redacted},
		{"escaped duplicate key", `{"detail":"dummy-api-key","\u0064etail":"safe"}`, "", `{"detail":"safe"}`, Available},
		{"surviving credential key", `{"apiKey":"safe","\u0061piKey":"dummy-api-key","message":"safe"}`, "safe", `{"message":"safe"}`, Available},
		{"protected array values", `{"detail":[{"apiKey":"dummy-api-key","ok":false},null,0,"",[],{}]}`, "", `{"detail":[{"ok":false},null,0,"",[],{}]}`, Available},
		{"ordinary transport names", `{"url":"https://ordinary.invalid","endpoint":"ordinary","headers":{"note":"sk-ordinary"},"request":{"body":"application"},"private":"application"}`, "", `{"url":"https://ordinary.invalid","endpoint":"ordinary","headers":{"note":"sk-ordinary"},"request":{"body":"application"},"private":"application"}`, Available},
		{"trailing value", `{"message":"safe"} {}`, "", "", Malformed},
		{"trailing malformed", `{"message":"safe"} trailing`, "", "", Malformed},
		{"normalized key and string", `{"\u0064etail":"\u0061","\ud800":"\ud801"}`, "", `{"detail":"a","�":"�"}`, Available},
	} {
		t.Run(tc.name, func(t *testing.T) {
			native := provider.NewAPICallError(provider.APICallErrorOptions{Data: json.RawMessage(tc.source)})
			projected, err := normalizeError(native, []string{"dummy-api-key"})
			require.NoError(t, err)
			assert.Equal(t, tc.message, projected.Message)
			details := readDetails(t, projected)
			assert.Equal(t, tc.state, details.State)
			if tc.want != "" {
				assert.JSONEq(t, tc.want, string(details.Value))
			}
			encoded, err := json.Marshal(projected)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "dummy-api-key")
			assert.Equal(t, json.RawMessage(tc.source), native.Data)
		})
	}
	native := provider.NewAPICallError(provider.APICallErrorOptions{Data: json.RawMessage(`{"code":9007199254740993,"detail":[1.2300e+2,1e30,-0]}`)})
	projected, err := normalizeError(native, nil)
	require.NoError(t, err)
	assert.Equal(t, "9007199254740993", string(projected.Code))
	for _, number := range []string{"9007199254740993", "1.2300e+2", "1e30", "-0"} {
		assert.Contains(t, string(readDetails(t, projected).Value), number)
	}
}

func readDetails(t *testing.T, value *NativeError) Component {
	t.Helper()
	require.NotNil(t, value)
	var component Component
	require.NoError(t, json.Unmarshal(value.Details, &component))
	return component
}
