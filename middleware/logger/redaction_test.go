package logger

import (
	"context"
	"log/slog"
	"strings"
	"testing"
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
