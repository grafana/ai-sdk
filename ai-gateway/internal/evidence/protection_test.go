package evidence

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectError_ProtectedFields(t *testing.T) {
	for _, field := range []string{"secretAccessKey", "privateKey", "googleCredentials", "x-goog-api-key", "x-amz-security-token", "X-Amz-Credential", "X-Amz-Signature", "x-access-token", "x-grafana-id", "apiKeyEnv", "access_token", "refresh_token", "oauthToken"} {
		t.Run(field, func(t *testing.T) {
			source, err := json.Marshal(map[string]any{"error": map[string]any{"message": "safe", "detail": map[string]any{field: "dummy-secret", "model": "sk-ordinary", "token": "ordinary"}}})
			require.NoError(t, err)
			native := provider.NewAPICallError(provider.APICallErrorOptions{Data: source})
			projected, invalid := projectError(native)
			require.False(t, invalid)
			require.NotNil(t, projected)
			assert.Equal(t, Available, projected.Details.State)
			assert.True(t, projected.Details.Redacted)
			assert.NotContains(t, string(projected.Details.Value), "dummy-secret")
			assert.Contains(t, string(projected.Details.Value), "sk-ordinary")
			assert.Contains(t, string(projected.Details.Value), `"token":"ordinary"`)
			assert.Equal(t, json.RawMessage(source), native.Data)
		})
	}
}

func TestProjectError_EscapedCredentialCode(t *testing.T) {
	for _, secret := range []string{"dummy<key>", "dummy&key", `dummy"key`, `dummy\key`} {
		t.Run(secret, func(t *testing.T) {
			source, err := json.Marshal(map[string]string{"message": "safe", "code": secret})
			require.NoError(t, err)
			projected, invalid := projectError(provider.NewAPICallError(provider.APICallErrorOptions{Data: source}), secret)
			require.False(t, invalid)
			assert.Nil(t, projected.Code)
			assert.Equal(t, Redacted, projected.Details.State)
		})
	}
}

func TestProjectError_DuplicateCredentialEcho(t *testing.T) {
	for _, source := range []string{`{"detail":"dummy-api-key","detail":"ordinary"}`, `{"detail":{"message":"dummy-api-key","message":"ordinary"}}`, `{"detail":"dummy-api-key","\u0064etail":"ordinary"}`} {
		projected, invalid := projectError(provider.NewAPICallError(provider.APICallErrorOptions{Data: json.RawMessage(source)}), "dummy-api-key")
		require.False(t, invalid)
		assert.Equal(t, Malformed, projected.Details.State)
		encoded, err := json.Marshal(projected)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "dummy-api-key")
	}
}

func TestProjectError_RedactedComponentBoundary(t *testing.T) {
	overhead, err := json.Marshal(Component{State: Available, Value: json.RawMessage(`{"detail":""}`), Redacted: true})
	require.NoError(t, err)
	for _, delta := range []int{0, 1} {
		t.Run(string(rune('0'+delta)), func(t *testing.T) {
			source := json.RawMessage(`{"apiKey":"dummy-secret","detail":"` + strings.Repeat("x", ComponentBytes-len(overhead)+delta) + `"}`)
			projected, invalid := projectError(provider.NewAPICallError(provider.APICallErrorOptions{Data: source}))
			require.False(t, invalid)
			encoded, err := json.Marshal(projected.Details)
			require.NoError(t, err)
			if delta == 0 {
				assert.Equal(t, Available, projected.Details.State)
				assert.Len(t, encoded, ComponentBytes)
			} else {
				assert.Equal(t, OverLimit, projected.Details.State)
			}
		})
	}
}
