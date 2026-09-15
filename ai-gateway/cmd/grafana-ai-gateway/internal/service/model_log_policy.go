package service

import (
	"context"
	"log/slog"
	"math"

	logmiddleware "github.com/grafana/ai-sdk/middleware/logger"
	"github.com/grafana/ai-sdk/provider"
)

var numericModelLogKeys = map[string]struct{}{
	"ai_sdk.duration_ms":                        {},
	"ai_sdk.duration_ns":                        {},
	"ai_sdk.error.status_code":                  {},
	"ai_sdk.usage.input_tokens.total":           {},
	"gen_ai.usage.input_tokens":                 {},
	"ai_sdk.usage.input_tokens.no_cache":        {},
	"ai_sdk.usage.input_tokens.cache_read":      {},
	"ai_sdk.usage.input_tokens.cache_write":     {},
	"ai_sdk.usage.output_tokens.total":          {},
	"gen_ai.usage.output_tokens":                {},
	"ai_sdk.usage.output_tokens.text":           {},
	"ai_sdk.usage.output_tokens.reasoning":      {},
	"ai_sdk.warnings.count":                     {},
	"ai_sdk.stream.parts.count":                 {},
	"ai_sdk.stream.parts.text_delta.count":      {},
	"ai_sdk.stream.parts.reasoning_delta.count": {},
	"ai_sdk.stream.parts.tool_call.count":       {},
	"ai_sdk.stream.parts.tool_result.count":     {},
	"ai_sdk.stream.parts.error.count":           {},
	"ai_sdk.stream.time_to_first_content_ms":    {},
}

// allowModelLogAttrs applies the Gateway-owned closed attribute policy before
// the reusable logger's default secret redactor. Unknown keys and values that
// are not valid for their fixed field are omitted rather than serialized.
func allowModelLogAttrs(attrs []slog.Attr) []slog.Attr {
	allowed := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		attr.Value = attr.Value.Resolve()
		if allowModelLogAttr(attr) {
			allowed = append(allowed, attr)
		}
	}
	return allowed
}

// closedModelLogRedactor keeps the Gateway allowlist fail-closed. The reusable
// logger's general panic fallback intentionally sees the original attributes;
// recovering here prevents that compatibility behavior from bypassing the
// Gateway policy if this delegate ever panics.
func closedModelLogRedactor(delegate logmiddleware.Redactor) logmiddleware.Redactor {
	return logmiddleware.RedactorFunc(func(ctx context.Context, event logmiddleware.EventKind, attrs []slog.Attr) (out []slog.Attr) {
		defer func() {
			if recover() != nil {
				out = nil
			}
		}()
		if delegate == nil {
			return nil
		}
		return delegate.RedactAttrs(ctx, event, allowModelLogAttrs(attrs))
	})
}

func allowModelLogAttr(attr slog.Attr) bool {
	if _, ok := numericModelLogKeys[attr.Key]; ok {
		if attr.Key == "ai_sdk.error.status_code" {
			return attr.Value.Kind() == slog.KindInt64 && attr.Value.Int64() >= 100 && attr.Value.Int64() <= 599
		}
		switch attr.Value.Kind() {
		case slog.KindInt64:
			return attr.Value.Int64() >= 0
		case slog.KindUint64:
			return true
		case slog.KindFloat64:
			value := attr.Value.Float64()
			return value >= 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
		default:
			return false
		}
	}

	switch attr.Key {
	case "ai_sdk.success", "ai_sdk.error.retryable":
		return attr.Value.Kind() == slog.KindBool
	case "ai_sdk.event":
		return stringIn(attr, "aisdk.model.generate.start", "aisdk.model.generate.finish", "aisdk.model.generate.error", "aisdk.model.stream.start", "aisdk.model.stream.finish", "aisdk.model.stream.error", "aisdk.model.stream.cancelled")
	case "ai_sdk.event.schema":
		return stringIn(attr, "1")
	case "ai_sdk.call.type":
		return stringIn(attr, "generate", "stream")
	case "ai_sdk.provider", "gen_ai.system":
		return stringIn(attr, "grafana")
	case "ai_sdk.outcome":
		return stringIn(attr, "success", "error", "cancelled", "timeout")
	case "ai_sdk.error.type":
		return stringIn(attr, "api_call_error", "context_canceled", "context_deadline_exceeded", "stream_part_error", "unknown")
	case "ai_sdk.finish_reason":
		return stringIn(attr,
			string(provider.FinishReasonStop),
			string(provider.FinishReasonLength),
			string(provider.FinishReasonContentFilter),
			string(provider.FinishReasonToolCalls),
			string(provider.FinishReasonError),
			string(provider.FinishReasonOther),
		)
	case "ai_sdk.call.id", "ai_sdk.model", "gen_ai.request.model", "correlation_id", "caller_service", "namespace", "region", "application":
		return attr.Value.Kind() == slog.KindString && boundedObservationValue(attr.Value.String()) != ""
	case "ai_sdk.warnings.types":
		values, ok := attr.Value.Any().([]string)
		if !ok || len(values) > 16 {
			return false
		}
		for _, value := range values {
			switch provider.WarningType(value) {
			case provider.WarnUnsupported, provider.WarnCompatibility, provider.WarnDeprecated, provider.WarnOther:
			default:
				return false
			}
		}
		return true
	default:
		return false
	}
}

func stringIn(attr slog.Attr, allowed ...string) bool {
	if attr.Value.Kind() != slog.KindString {
		return false
	}
	value := attr.Value.String()
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
