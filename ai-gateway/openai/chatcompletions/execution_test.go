package chatcompletions

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionAttribution(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, failed := range []bool{false, true} {
			t.Run(map[bool]string{false: "unary", true: "stream"}[streaming]+map[bool]string{false: " success", true: " failure"}[failed], func(t *testing.T) {
				native := provider.ProviderMetadata{"gateway": json.RawMessage(`{"native":true}`), "vendor": json.RawMessage(`{"detail":1}`)}
				nativeErr := provider.NewAPICallError(provider.APICallErrorOptions{Message: "bad key test-secret-key", StatusCode: 429})
				f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					if failed {
						return nil, nativeErr
					}
					r := textResult()
					r.ProviderMetadata = native
					return r, nil
				}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					if failed {
						return nil, nativeErr
					}
					parts := textParts()
					parts[len(parts)-1].ProviderMetadata = native
					return partsStream(parts...), nil
				}}
				runtime, err := execution.New(execution.Config{Limits: execution.DefaultLimits, ConfiguredAPIKeys: []string{"test-secret-key"}, Selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (execution.Selection, error) {
					return execution.ConfiguredSelection("canonical", f, []execution.ExpectedAttempt{{ModelID: "alias", CanonicalID: "canonical", Destination: catalog.ConfiguredCandidate{Provider: "vendor", ModelID: "native-model", ProviderInstance: "instance"}}}), nil
				}})
				require.NoError(t, err)
				h, err := New(runtime)
				require.NoError(t, err)
				body := basic
				if streaming {
					body = strings.TrimSuffix(body, "}") + `,"stream":true}`
				}
				req := httptest.NewRequest("POST", Path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
				text := w.Body.String()
				assert.Contains(t, text, `"execution"`)
				assert.Contains(t, text, `"requestedModelId":"alias"`)
				assert.Contains(t, text, `"providerInstance":"instance"`)
				assert.NotContains(t, text, "test-secret-key")
				if failed {
					assert.Equal(t, 429, w.Code)
					assert.Contains(t, text, "[REDACTED]")
				} else {
					assert.Contains(t, text, `"nativeMetadata":{"native":true}`)
					assert.Contains(t, text, `"vendor":{"detail":1}`)
				}
			})
		}
	}
}
