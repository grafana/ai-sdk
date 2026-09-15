package service

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"testing"

	logmiddleware "github.com/grafana/ai-sdk/middleware/logger"
	"github.com/stretchr/testify/assert"
)

func TestAllowModelLogAttrs_ClosedPolicy(t *testing.T) {
	allowed := []slog.Attr{
		slog.String("ai_sdk.event", "aisdk.model.stream.finish"),
		slog.String("ai_sdk.event.schema", "1"),
		slog.String("ai_sdk.call.id", "call-id"),
		slog.String("ai_sdk.call.type", "stream"),
		slog.String("ai_sdk.provider", "grafana"),
		slog.String("gen_ai.system", "grafana"),
		slog.String("ai_sdk.model", "grafana/assistant"),
		slog.String("gen_ai.request.model", "grafana/assistant"),
		slog.Int64("ai_sdk.duration_ns", 100),
		slog.Float64("ai_sdk.duration_ms", 0.1),
		slog.String("ai_sdk.outcome", "success"),
		slog.Bool("ai_sdk.success", true),
		slog.String("ai_sdk.finish_reason", "stop"),
		slog.Int("ai_sdk.usage.input_tokens.total", 1),
		slog.Int("gen_ai.usage.input_tokens", 1),
		slog.Int("ai_sdk.warnings.count", 2),
		slog.Any("ai_sdk.warnings.types", []string{"unsupported", "deprecated"}),
		slog.Int("ai_sdk.stream.parts.count", 3),
		slog.Int("ai_sdk.stream.parts.text_delta.count", 2),
		slog.Float64("ai_sdk.stream.time_to_first_content_ms", 2.5),
		slog.String("correlation_id", "correlation"),
		slog.String("caller_service", "service"),
		slog.String("namespace", "stack-1"),
		slog.String("region", "us-central1"),
		slog.String("application", "ai-gateway"),
	}
	assert.Equal(t, allowed, allowModelLogAttrs(allowed))
}

func TestAllowModelLogAttrs_DropsProviderAndFutureValues(t *testing.T) {
	privateValues := []string{
		"raw-secret", "https://provider.invalid", "backend-private", "response-private",
		"raw-finish-private", "warning-feature-private", "warning-message-private",
		"provider-option-private", "provider-metadata-private", "payload-private",
	}
	attrs := []slog.Attr{
		slog.String("ai_sdk.error.message", privateValues[0]),
		slog.String("ai_sdk.error.url", privateValues[1]),
		slog.String("ai_sdk.transport.model", privateValues[2]),
		slog.String("ai_sdk.response.id", privateValues[3]),
		slog.String("ai_sdk.finish_reason.raw", privateValues[4]),
		slog.String("ai_sdk.warnings.features", privateValues[5]),
		slog.String("ai_sdk.warnings.messages", privateValues[6]),
		slog.String("ai_sdk.request.provider_options", privateValues[7]),
		slog.String("ai_sdk.provider_metadata", privateValues[8]),
		slog.String("ai_sdk.response.content", privateValues[9]),
		slog.String("future.unknown", "future-private"),
		slog.String("ai_sdk.error.type.go", "private.Error"),
		slog.Group("nested", slog.String("correlation_id", "smuggled")),
	}
	assert.Empty(t, allowModelLogAttrs(attrs))
}

func TestAllowModelLogAttrs_DropsInvalidValuesUnderAllowedKeys(t *testing.T) {
	attrs := []slog.Attr{
		slog.String("ai_sdk.event", "secret-event"),
		slog.String("ai_sdk.call.type", "private-call"),
		slog.String("ai_sdk.provider", "anthropic"),
		slog.String("ai_sdk.outcome", "provider-private"),
		slog.String("ai_sdk.error.type", "private.Error"),
		slog.String("ai_sdk.finish_reason", "provider-finish"),
		slog.Int("ai_sdk.error.status_code", 999),
		slog.Uint64("ai_sdk.error.status_code", 503),
		slog.Int("ai_sdk.usage.input_tokens.total", -1),
		slog.Float64("ai_sdk.duration_ms", -1),
		slog.Float64("ai_sdk.duration_ms", math.Inf(1)),
		slog.Float64("ai_sdk.duration_ms", math.NaN()),
		slog.String("correlation_id", strings.Repeat("x", maxObservationValueLen+1)),
		slog.String("caller_service", "line\nbreak"),
		slog.Any("ai_sdk.warnings.types", []string{"unsupported", "provider-private"}),
	}
	assert.Empty(t, allowModelLogAttrs(attrs))
}

func TestClosedModelLogRedactor_DelegatePanicFailsClosed(t *testing.T) {
	redactor := closedModelLogRedactor(logmiddleware.RedactorFunc(func(context.Context, logmiddleware.EventKind, []slog.Attr) []slog.Attr {
		panic("private-redactor-panic")
	}))
	attrs := redactor.RedactAttrs(context.Background(), logmiddleware.EventGenerateError, []slog.Attr{
		slog.String("ai_sdk.event", "aisdk.model.generate.error"),
		slog.String("ai_sdk.error.message", "private-error"),
	})
	assert.Empty(t, attrs)
	assert.Empty(t, closedModelLogRedactor(nil).RedactAttrs(context.Background(), logmiddleware.EventGenerateError, nil))
}
