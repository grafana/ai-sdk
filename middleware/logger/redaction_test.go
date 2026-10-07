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

func TestDefaultRedactor_RedactsNestedStructuredValues(t *testing.T) {
	attrs := []slog.Attr{
		slog.Any("headers", map[string]any{
			"Authorization": "Bearer secret-token",
			"accessToken":   "secret-camel-token",
			"nested": map[string]any{
				"x-api-key": "secret-key",
				"safe":      "visible",
			},
			"items": []any{map[string]any{"password": "secret-password"}},
		}),
		slog.Group("group",
			slog.String("cookie", "secret-cookie"),
			slog.String("safe", "visible"),
		),
	}

	redacted := DefaultRedactor().RedactAttrs(context.Background(), EventGenerateStart, attrs)
	json := recordsJSONForAttrs(t, redacted)
	for _, secret := range []string{"secret-token", "secret-camel-token", "secret-key", "secret-password", "secret-cookie"} {
		if strings.Contains(json, secret) {
			t.Fatalf("redacted attrs leaked %q: %s", secret, json)
		}
	}
	if !strings.Contains(json, redactedValue) || !strings.Contains(json, "visible") {
		t.Fatalf("expected redaction marker and safe value: %s", json)
	}
}

func TestDefaultRedactor_RedactsPluralTokenSecretsButKeepsUsageCounters(t *testing.T) {
	attrs := []slog.Attr{
		slog.String("access_tokens", "secret-token"),
		slog.Int("ai_sdk.usage.input_tokens.total", 12),
	}

	redacted := DefaultRedactor().RedactAttrs(context.Background(), EventGenerateStart, attrs)
	handler := newTestHandler()
	slog.New(handler).LogAttrs(context.Background(), slog.LevelInfo, "test", redacted...)
	got := handler.Records()[0].AttrsMap()
	assertAttr(t, got, "access_tokens", redactedValue)
	assertAttr(t, got, "ai_sdk.usage.input_tokens.total", int64(12))
}

func TestDefaultRedactorWithExtraKeys_RedactsAdditionalPatterns(t *testing.T) {
	attrs := []slog.Attr{slog.String("x-internal-signature", "secret-signature")}
	redacted := DefaultRedactorWithExtraKeys("x-internal-signature").RedactAttrs(context.Background(), EventGenerateStart, attrs)
	handler := newTestHandler()
	slog.New(handler).LogAttrs(context.Background(), slog.LevelInfo, "test", redacted...)
	assertAttr(t, handler.Records()[0].AttrsMap(), "x-internal-signature", redactedValue)
}

func TestDefaultRedactor_DoesNotRewriteOpaqueStrings(t *testing.T) {
	attrs := []slog.Attr{slog.String("payload", `{"authorization":"secret"}`)}
	redacted := DefaultRedactor().RedactAttrs(context.Background(), EventGenerateStart, attrs)
	got := redacted[0].Value.String()
	if !strings.Contains(got, "secret") {
		t.Fatalf("opaque string should be left unchanged, got %q", got)
	}
}

func TestRedactorFunc_RemovesAttrs(t *testing.T) {
	redactor := RedactorFunc(func(_ context.Context, _ EventKind, attrs []slog.Attr) []slog.Attr {
		out := make([]slog.Attr, 0, len(attrs))
		for _, attr := range attrs {
			if attr.Key == "remove" {
				continue
			}
			out = append(out, attr)
		}
		return out
	})
	redacted := redactor.RedactAttrs(context.Background(), EventGenerateStart, []slog.Attr{
		slog.String("keep", "yes"),
		slog.String("remove", "no"),
	})
	if len(redacted) != 1 || redacted[0].Key != "keep" {
		t.Fatalf("unexpected attrs: %#v", redacted)
	}
}

func recordsJSONForAttrs(t *testing.T, attrs []slog.Attr) string {
	t.Helper()
	handler := newTestHandler()
	logger := slog.New(handler)
	logger.LogAttrs(context.Background(), slog.LevelInfo, "test", attrs...)
	return handler.JSON(t)
}

func TestJSONAttr_BYOKSanitization(t *testing.T) {
	const rawOptions = `{"byok":{"openai":[{"unfamiliar":{"nested":"dummy-credential"}}]},"ordinary":true,"providerTimeouts":{"byok":{"openai":5000}}}`
	options := provider.ProviderOptions{"gateway": provider.RawProviderOption{Raw: json.RawMessage(rawOptions)}}
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"typed options", options},
		{"raw body", json.RawMessage(`{"providerOptions":{"gateway":` + rawOptions + `},"prompt":"application gateway.byok dummy-application"}`)},
		{"message options", []provider.Message{{Role: provider.RoleUser, ProviderOptions: options}}},
		{"tool options", []provider.Tool{{ProviderOptions: options}}},
		{"opaque options", json.RawMessage(`{"providerOptions":"dummy-credential","ordinary":true}`)},
		{"malformed gateway", json.RawMessage(`{"providerOptions":{"gateway":["dummy-credential"]},"ordinary":true}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := json.Marshal(tc.value)
			require.NoError(t, err)
			for _, limit := range []int{1, 8192} {
				attr, ok := jsonAttr("capture", tc.value, CaptureOptions{MaxJSONBytes: limit})
				require.True(t, ok)
				data := recordsJSONForAttrs(t, []slog.Attr{attr})
				assert.NotContains(t, data, "dummy-credential")
				if limit > 1 {
					assert.Contains(t, data, "ordinary")
					assert.NotContains(t, data, `"Raw"`)
					assert.Contains(t, data, redactedValue)
					if tc.name == "raw body" {
						assert.Contains(t, data, "application gateway.byok dummy-application")
					}
				}
			}
			after, err := json.Marshal(tc.value)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func TestCapture_BYOKPreservesOrdinaryData(t *testing.T) {
	capture := CaptureOptions{ToolOutputs: true, ProviderOptions: true, MaxJSONBytes: 8192}
	attrs := streamPartCaptureAttrs(provider.StreamPart{
		Type: provider.PartToolResult, Result: json.RawMessage(`{"gateway":"west","byok":"application-value"}`),
	}, capture)
	attrs = append(attrs, requestCaptureAttrs(provider.CallOptions{ProviderOptions: provider.ProviderOptions{
		"gateway": provider.RawProviderOption{Raw: json.RawMessage(`{"providerTimeouts":{"byok":{"openai":5000}}}`)},
	}}, capture)...)
	data := recordsJSONForAttrs(t, DefaultRedactor().RedactAttrs(context.Background(), EventStreamPart, attrs))
	assert.Contains(t, data, `"gateway":"west"`)
	assert.Contains(t, data, `"byok":"application-value"`)
	assert.Contains(t, data, `"byok":{"openai":5000}`)
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
			}, Options{
				Logger: slog.New(handler), Capture: CaptureOptions{RequestBody: true, MaxJSONBytes: 8192},
				Redactor: RedactorFunc(func(_ context.Context, _ EventKind, attrs []slog.Attr) []slog.Attr { return attrs }),
			})
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

func TestRequestBodyCapture_UnsafeRepresentations(t *testing.T) {
	for _, body := range []json.RawMessage{
		json.RawMessage(`"{\"providerOptions\":{\"gateway\":{\"byok\":\"dummy-key\"}}}"`),
		json.RawMessage(`[{"providerOptions":{"gateway":{"byok":"dummy-key"}}}]`),
		json.RawMessage(`{"providerOptions":{"gateway":{"byok":"dummy-incomplete`),
	} {
		attrs := appendRequestBodyAttr(nil, body, CaptureOptions{MaxJSONBytes: 8192})
		require.Equal(t, []slog.Attr{slog.String("ai_sdk.serialization_error", "ai_sdk.request.body")}, attrs)
	}
}

func TestJSONAttr_UnsafeRepresentations(t *testing.T) {
	cycle := map[string]any{}
	cycle["self"] = cycle
	for _, value := range []any{cycle, panickingBYOKCapture{}, json.RawMessage(`{"incomplete":`)} {
		attr, ok := jsonAttr("capture", value, CaptureOptions{MaxJSONBytes: 8192})
		assert.False(t, ok)
		assert.Equal(t, slog.Attr{}, attr)
	}
}
