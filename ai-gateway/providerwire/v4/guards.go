package v4

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

var (
	// ErrGuardDenied reports an explicit policy denial.
	ErrGuardDenied = errors.New("providerwire v4: guard denied")
	// ErrGuardFailed reports a closed guard failure.
	ErrGuardFailed = errors.New("providerwire v4: guard failed")
	// ErrGuardUnsupported reports input outside the guarded content contract.
	ErrGuardUnsupported = errors.New("providerwire v4: guard input unsupported")
)

// Guard evaluates one logical request, outside provider selection and fallback.
// Implementations own admission and service-failure policy; output transformations
// must not release the original content.
type Guard interface {
	Acquire(context.Context) (release func(), err error)
	Preflight(context.Context, string, provider.CallOptions) (provider.CallOptions, error)
	Postflight(context.Context, string, provider.CallOptions, []provider.ContentPart) error
}

func (h *handler) serveGuarded(w http.ResponseWriter, ctx context.Context, resolved catalog.ResolvedModel, options provider.CallOptions, mode executionMode) {
	ctx, cancel := context.WithTimeout(ctx, h.limits.ModelDuration)
	defer cancel()
	release, err := h.guard.Acquire(ctx)
	if err != nil {
		h.writeSafeError(w, safeErrorFromGuard(err))
		return
	}
	if release == nil {
		h.writeSafeError(w, safeError{category: safeFailedDependency})
		return
	}
	defer release()
	options, err = h.guard.Preflight(ctx, resolved.ID, options)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		h.writeSafeError(w, safeErrorFromGuard(err))
		return
	}
	if mode == executionStreaming {
		h.serveGuardedStream(w, ctx, resolved, options)
		return
	}
	result, err := h.invokeModel(ctx, resolved.Model, options)
	if err != nil {
		h.writeSafeError(w, safeErrorFromProvider(err))
		return
	}
	if !unarySuccessPreflight(result, h.guardRetainedBytes/16) {
		h.writeSafeError(w, safeError{category: safeFailedDependency})
		return
	}
	mapped, err := mapUnarySuccess(result, h.limits.UnaryResponseBytes)
	if err != nil {
		h.writeSafeError(w, safeError{category: safeInternal})
		return
	}
	body, ok := encodeUnarySuccess(mapped, h.limits.UnaryResponseBytes)
	if !ok {
		h.writeSafeError(w, safeError{category: safeInternal})
		return
	}
	output, err := guardedUnaryOutput(mapped)
	if err != nil || int64(len(body))*4 > h.guardRetainedBytes/2 {
		h.writeSafeError(w, safeError{category: safeFailedDependency})
		return
	}
	err = h.guard.Postflight(ctx, resolved.ID, options, output)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		h.writeSafeError(w, safeErrorFromGuard(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func guardedUnaryOutput(value unarySuccess) ([]provider.ContentPart, error) {
	parts := make([]provider.ContentPart, 0, len(value.Content))
	for _, content := range value.Content {
		switch part := content.(type) {
		case unaryTextPart:
			parts = append(parts, provider.TextPart(part.Text))
		case reasoningTextPart:
			if part.Metadata != nil && guardedReasoningMetadata(*part.Metadata) != nil {
				return nil, ErrGuardFailed
			}
			parts = append(parts, provider.ReasoningPart(part.Text))
		case unaryToolCall:
			if !json.Valid([]byte(part.Input)) {
				return nil, ErrGuardFailed
			}
			parts = append(parts, provider.ToolCallPart(part.ToolCallID, part.ToolName, json.RawMessage(part.Input)))
		default:
			return nil, ErrGuardFailed
		}
	}
	return parts, nil
}

func guardedReasoningMetadata(metadata provider.ProviderMetadata) error {
	for namespace, raw := range metadata {
		var keys []string
		switch namespace {
		case "anthropic":
			keys = []string{"redactedData"}
		case "bedrock", "amazonBedrock":
			keys = []string{"redactedData", "redactedContent"}
		case "openai":
			keys = []string{"reasoningEncryptedContent"}
		default:
			continue
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return ErrGuardFailed
		}
		for _, key := range keys {
			if value, present := fields[key]; present && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return ErrGuardFailed
			}
		}
	}
	return nil
}

func safeErrorFromGuard(err error) safeError {
	switch {
	case errors.Is(err, ErrGuardDenied):
		return safeError{category: safeForbidden}
	case errors.Is(err, ErrGuardUnsupported):
		return safeError{category: safeInvalidRequest}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return safeErrorFromProvider(err)
	default:
		return safeError{category: safeFailedDependency}
	}
}
