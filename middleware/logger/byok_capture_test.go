package logger

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONAttr_BYOKSanitization(t *testing.T) {
	const options = `{"gateway":{"byok":{"openai":[{"unfamiliar":{"nested":"dummy-credential"}}]},"ordinary":true},"openai":{"store":false}}`
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"raw options", json.RawMessage(options)},
		{"typed options", provider.ProviderOptions{"gateway": provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(`{"byok":{"openai":[{"future":"dummy-credential"}]},"ordinary":true}`)}}},
		{"byte options", []byte(options)},
		{"malformed gateway array", map[string]any{"ordinary": true, "gateway": []any{"dummy-credential"}}},
		{"raw body", json.RawMessage(`{"providerOptions":` + options + `,"prompt":"application gateway.byok dummy-application"}`)},
		{"nested bytes", map[string]any{"providerOptions": []byte(options)}},
		{"error request", map[string]any{"providerOptions": map[string]any{"gateway": json.RawMessage(`{"byok":[{"unknown":"dummy-credential"}],"ordinary":true}`)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := json.Marshal(tc.value)
			require.NoError(t, err)
			for _, limit := range []int{1, 8192} {
				attr, ok := jsonAttr("capture", tc.value, CaptureOptions{MaxJSONBytes: limit})
				require.True(t, ok)
				data, err := json.Marshal(attr.Value.Any())
				require.NoError(t, err)
				assert.NotContains(t, string(data), "dummy-credential")
				if limit > 1 {
					assert.Contains(t, string(data), "ordinary")
					assert.NotContains(t, string(data), `"Raw"`)
					assert.Contains(t, string(data), redactedValue)
					if tc.name == "raw body" {
						assert.Contains(t, string(data), "application gateway.byok dummy-application")
					}
				}
			}
			after, err := json.Marshal(tc.value)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func TestMiddleware_BYOKErrorBodyCapture(t *testing.T) {
	body := json.RawMessage(`{"providerOptions":{"gateway":{"byok":{"openai":[{"future":"dummy-error-key"}]},"ordinary":true}}}`)
	before := string(body)
	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			handler := newTestHandler()
			failure := provider.NewAPICallError(provider.APICallErrorOptions{Message: "safe failure", StatusCode: 400, RequestBodyValues: body})
			model := Wrap(&mockModel{
				generateFunc: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, failure },
				streamFunc:   func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, failure },
			}, Options{Logger: slog.New(handler), Capture: CaptureOptions{RequestBody: true, MaxJSONBytes: 8192}})
			var err error
			if mode == "stream" {
				_, err = model.DoStream(context.Background(), provider.CallOptions{})
			} else {
				_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
			}
			require.ErrorIs(t, err, failure)
			captured := handler.JSON(t)
			assert.Contains(t, captured, "[REDACTED]")
			assert.Contains(t, captured, "ordinary")
			assert.NotContains(t, captured, "dummy-error-key")
			assert.Equal(t, json.RawMessage(before), failure.RequestBodyValues)
		})
	}
}

type panickingBYOKCapture struct{}

func (panickingBYOKCapture) MarshalJSON() ([]byte, error) { panic("dummy-private-capture") }

func TestJSONAttr_BYOKUnsafeRepresentations(t *testing.T) {
	for _, key := range []string{"ai_sdk.request.body", "ai_sdk.request.provider_options"} {
		for _, value := range []any{json.RawMessage(`"{\"gateway\":{\"byok\":\"dummy-key\"}}"`), json.RawMessage(`[{"gateway":{"byok":"dummy-key"}}]`)} {
			_, ok := jsonAttr(key, value, CaptureOptions{MaxJSONBytes: 8192})
			assert.False(t, ok)
		}
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	for _, value := range []any{cycle, panickingBYOKCapture{}, json.RawMessage(`{"gateway":{"byok":"dummy-incomplete`), []byte(strings.Repeat("x", maxCaptureBytes+1))} {
		attr, ok := jsonAttr("capture", value, CaptureOptions{MaxJSONBytes: 8192})
		assert.False(t, ok)
		assert.Equal(t, slog.Attr{}, attr)
	}
}
