package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntime_RequestAccountSummaryRedaction(t *testing.T) {
	runtime, err := New(Config{Limits: testLimits(), ConfiguredAPIKeys: []string{"configured-secret"}, Selector: func(_ context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (Selection, error) {
		keys := []string{id}
		selected := AccountSelection("native/model", &generateModel{modelID: "model"}, accountAttempts("native", "model", 1), keys...)
		keys[0] = "mutated-secret"
		return selected, nil
	}})
	require.NoError(t, err)
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			key := fmt.Sprintf("request-secret-%d!", i)
			call, err := runtime.Call(t.Context(), key, provider.CallOptions{})
			require.NoError(t, err)
			defer call.Close()
			source := provider.NewAPICallError(provider.APICallErrorOptions{Message: "configured-secret " + key + " unrelated-secret", StatusCode: 401})
			got := call.View(nil).Current(source)
			require.NotNil(t, got)
			assert.Equal(t, "[REDACTED] [REDACTED] unrelated-secret", got.Message)
			assert.Contains(t, source.Error(), key)
			assert.Equal(t, []string{"configured-secret"}, runtime.redactor.keys)
		})
	}
}

func TestAccountSelection_SecretsArePrivate(t *testing.T) {
	selected := AccountSelection("native/model", &generateModel{modelID: "model"}, nil, "account-secret")
	raw, err := json.Marshal(selected)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(raw), "account-secret"))
}
