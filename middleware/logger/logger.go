package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"sort"
	"time"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

// Middleware returns a language-model middleware that logs provider calls.
func Middleware(opts Options) middleware.Middleware {
	l := &modelLogger{opts: normalizeOptions(opts)}
	return middleware.Middleware{
		WrapGenerate: l.wrapGenerate,
		WrapStream:   l.wrapStream,
	}
}

// Wrap applies Middleware(opts) to base.
func Wrap(base provider.LanguageModel, opts Options) provider.LanguageModel {
	return middleware.Wrap(middleware.WrapOptions{
		Model:      base,
		Middleware: []middleware.Middleware{Middleware(opts)},
	})
}

const eventSchemaVersion = "1"

const (
	outcomeSuccess   = "success"
	outcomeError     = "error"
	outcomeCancelled = "cancelled"
	outcomeTimeout   = "timeout"
)

type modelLogger struct {
	opts normalizedOptions
}

func (l *modelLogger) wrapGenerate(ctx context.Context, p middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
	started := l.opts.clock()
	callID := newCallID()
	l.log(ctx, EventGenerateStart, l.opts.level, append(
		commonAttrs(callID, "generate", p.Model),
		requestSummaryAttrs(p.Params, l.opts.capture)...,
	)...)

	result, err := p.DoGenerate(ctx)
	duration := l.opts.clock().Sub(started)
	if err != nil {
		attrs := append(commonAttrs(callID, "generate", p.Model), terminalAttrs(duration, outcomeError)...)
		attrs = append(attrs, errorAttrs(err, l.opts.capture)...)
		l.log(ctx, EventGenerateError, l.opts.errorLevel, attrs...)
		return nil, err
	}

	attrs := append(terminalCommonAttrs(callID, "generate", p.Model, responseMetadataFromGenerate(result)), terminalAttrs(duration, outcomeSuccess)...)
	attrs = append(attrs, generateResultAttrs(result, l.opts.capture)...)
	l.log(ctx, EventGenerateFinish, l.opts.level, attrs...)
	return result, nil
}

func (l *modelLogger) log(ctx context.Context, event EventKind, level slog.Level, attrs ...slog.Attr) {
	all := make([]slog.Attr, 0, 2+len(attrs)+len(l.opts.attrs)+4)
	all = append(all,
		slog.String("ai_sdk.event", string(event)),
		slog.String("ai_sdk.event.schema", eventSchemaVersion),
	)
	all = append(all, attrs...)
	all = append(all, l.opts.attrs...)
	all = append(all, l.dynamicAttrs(ctx)...)
	all = l.redactAttrs(ctx, event, all)
	l.logAttrs(ctx, level, string(event), all...)
}

func (l *modelLogger) dynamicAttrs(ctx context.Context) (attrs []slog.Attr) {
	if l.opts.dynamicAttrs == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			attrs = []slog.Attr{slog.String("ai_sdk.serialization_error", fmt.Sprintf("dynamic attrs panic: %v", recovered))}
		}
	}()
	return l.opts.dynamicAttrs(ctx)
}

func (l *modelLogger) redactAttrs(ctx context.Context, event EventKind, attrs []slog.Attr) (out []slog.Attr) {
	defer func() {
		if recovered := recover(); recovered != nil {
			out = DefaultRedactor().RedactAttrs(ctx, event, attrs)
			out = append(out, slog.String("ai_sdk.serialization_error", fmt.Sprintf("redactor panic: %v", recovered)))
		}
	}()
	return l.opts.redactor.RedactAttrs(ctx, event, attrs)
}

func (l *modelLogger) logAttrs(ctx context.Context, level slog.Level, message string, attrs ...slog.Attr) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logHandlerPanic(ctx, l.opts.logger, recovered)
		}
	}()
	l.opts.logger.LogAttrs(ctx, level, message, attrs...)
}

func logHandlerPanic(ctx context.Context, logger *slog.Logger, recovered any) {
	defaultLogger := slog.Default()
	if logger.Handler() != defaultLogger.Handler() {
		logged := false
		func() {
			defer func() { _ = recover() }()
			defaultLogger.LogAttrs(ctx, slog.LevelError, "logger: slog handler panic",
				slog.Any("panic", recovered),
			)
			logged = true
		}()
		if logged {
			return
		}
	}
	_, _ = fmt.Fprintf(os.Stderr, "logger: slog handler panic: %v\n", recovered)
}

func commonAttrs(callID, callType string, model provider.LanguageModel) []slog.Attr {
	return modelIdentityAttrs(callID, callType, modelIdentityFromModel(model), modelIdentity{})
}

func terminalCommonAttrs(callID, callType string, model provider.LanguageModel, response provider.ResponseMetadata) []slog.Attr {
	return modelIdentityAttrs(callID, callType, resolvedModelIdentity(modelIdentityFromModel(model), modelIdentityFromResponse(response)), modelIdentityFromModel(model))
}

type modelIdentity struct {
	provider string
	model    string
}

func modelIdentityFromModel(model provider.LanguageModel) modelIdentity {
	if model == nil {
		return modelIdentity{}
	}
	return modelIdentity{provider: model.Provider(), model: model.ModelID()}
}

func modelIdentityFromResponse(meta provider.ResponseMetadata) modelIdentity {
	return modelIdentity{provider: meta.Provider, model: meta.ModelID}
}

func (m modelIdentity) complete() bool { return m.provider != "" && m.model != "" }

func resolvedModelIdentity(seed, response modelIdentity) modelIdentity {
	if response.complete() {
		return response
	}
	return seed
}

func modelIdentityAttrs(callID, callType string, identity, transport modelIdentity) []slog.Attr {
	attrs := []slog.Attr{
		slog.String("ai_sdk.call.id", callID),
		slog.String("ai_sdk.call.type", callType),
	}
	if identity.provider != "" {
		attrs = append(attrs,
			slog.String("ai_sdk.provider", identity.provider),
			slog.String("gen_ai.system", identity.provider),
		)
	}
	if identity.model != "" {
		attrs = append(attrs, slog.String("ai_sdk.model", identity.model))
	}
	if transport.model != "" {
		attrs = append(attrs, slog.String("gen_ai.request.model", transport.model))
	} else if identity.model != "" {
		attrs = append(attrs, slog.String("gen_ai.request.model", identity.model))
	}
	if transport.complete() && identity.complete() && transport != identity {
		attrs = append(attrs,
			slog.String("ai_sdk.transport.provider", transport.provider),
			slog.String("ai_sdk.transport.model", transport.model),
		)
	}
	return attrs
}

func terminalAttrs(duration time.Duration, outcome string) []slog.Attr {
	return []slog.Attr{
		slog.Float64("ai_sdk.duration_ms", durationMs(duration)),
		slog.Int64("ai_sdk.duration_ns", duration.Nanoseconds()),
		slog.String("ai_sdk.outcome", outcome),
		slog.Bool("ai_sdk.success", outcome == outcomeSuccess),
	}
}

func durationMs(duration time.Duration) float64 {
	return float64(duration.Nanoseconds()) / float64(time.Millisecond)
}

func newCallID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err == nil {
		return hex.EncodeToString(data[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func requestSummaryAttrs(params provider.CallOptions, capture CaptureOptions) []slog.Attr {
	attrs := make([]slog.Attr, 0, 20)
	attrs = append(attrs, slog.Int("ai_sdk.request.prompt.messages.count", len(params.Prompt)))
	attrs = append(attrs, slog.Int("ai_sdk.request.tools.count", len(params.Tools)))
	attrs = append(attrs, slog.Int("ai_sdk.request.stop_sequences.count", len(params.StopSequences)))
	attrs = append(attrs, slog.Bool("ai_sdk.request.include_raw_chunks", params.IncludeRawChunks))
	if params.MaxOutputTokens != nil {
		attrs = append(attrs, slog.Int("ai_sdk.request.max_output_tokens", *params.MaxOutputTokens))
	}
	if params.Temperature != nil {
		attrs = append(attrs, slog.Float64("ai_sdk.request.temperature", *params.Temperature))
	}
	if params.TopP != nil {
		attrs = append(attrs, slog.Float64("ai_sdk.request.top_p", *params.TopP))
	}
	if params.TopK != nil {
		attrs = append(attrs, slog.Int("ai_sdk.request.top_k", *params.TopK))
	}
	if params.PresencePenalty != nil {
		attrs = append(attrs, slog.Float64("ai_sdk.request.presence_penalty", *params.PresencePenalty))
	}
	if params.FrequencyPenalty != nil {
		attrs = append(attrs, slog.Float64("ai_sdk.request.frequency_penalty", *params.FrequencyPenalty))
	}
	if params.Seed != nil {
		attrs = append(attrs, slog.Int("ai_sdk.request.seed", *params.Seed))
	}
	if params.Reasoning != nil {
		attrs = append(attrs, slog.String("ai_sdk.request.reasoning_effort", string(*params.Reasoning)))
	}
	if params.ToolChoice != nil {
		attrs = append(attrs, slog.String("ai_sdk.request.tool_choice.type", string(params.ToolChoice.Type)))
	}
	if params.ResponseFormat != nil {
		attrs = append(attrs, slog.String("ai_sdk.request.response_format.type", string(params.ResponseFormat.Type)))
	}

	attrs = append(attrs, requestCaptureAttrs(params, capture)...)
	return attrs
}

func requestCaptureAttrs(params provider.CallOptions, capture CaptureOptions) []slog.Attr {
	attrs := make([]slog.Attr, 0, 6)
	if capture.Inputs && len(params.Prompt) > 0 {
		attrs = appendJSONAttr(attrs, "ai_sdk.request.prompt", sanitizeMessages(params.Prompt, capture), capture)
	}
	if capture.ToolInputs && len(params.Tools) > 0 {
		attrs = appendJSONAttr(attrs, "ai_sdk.request.tools", sanitizeTools(params.Tools, capture), capture)
	}
	if capture.Headers && len(params.Headers) > 0 {
		attrs = append(attrs, slog.Any("ai_sdk.request.headers", cloneStringMap(params.Headers)))
	}
	if capture.ProviderOptions && len(params.ProviderOptions) > 0 {
		attrs = appendJSONAttr(attrs, "ai_sdk.request.provider_options", params.ProviderOptions, capture)
	}
	return attrs
}

func generateResultAttrs(result *provider.GenerateResult, capture CaptureOptions) []slog.Attr {
	if result == nil {
		return nil
	}
	attrs := make([]slog.Attr, 0, 16)
	attrs = append(attrs, finishReasonAttrs(result.FinishReason)...)
	attrs = append(attrs, usageAttrs(result.Usage)...)
	attrs = append(attrs, warningAttrs(result.Warnings)...)
	if result.Response != nil {
		attrs = append(attrs, responseMetadataAttrs(result.Response.ResponseMetadata)...)
		if capture.Headers && len(result.Response.Headers) > 0 {
			attrs = append(attrs, slog.Any("ai_sdk.response.headers", cloneStringMap(result.Response.Headers)))
		}
		if capture.ResponseBody && len(result.Response.Body) > 0 {
			attrs = appendJSONAttr(attrs, "ai_sdk.response.body", result.Response.Body, capture)
		}
	}
	if result.Request != nil && capture.RequestBody && len(result.Request.Body) > 0 {
		attrs = appendJSONAttr(attrs, "ai_sdk.request.body", result.Request.Body, capture)
	}
	if shouldCaptureGeneratedContent(capture) && len(result.Content) > 0 {
		attrs = appendJSONAttr(attrs, "ai_sdk.response.content", sanitizeGenerateContent(result.Content, capture), capture)
	}
	if capture.ProviderMetadata && len(result.ProviderMetadata) > 0 {
		attrs = appendJSONAttr(attrs, "ai_sdk.provider_metadata", result.ProviderMetadata, capture)
	}
	return attrs
}

func responseMetadataFromGenerate(result *provider.GenerateResult) provider.ResponseMetadata {
	if result == nil || result.Response == nil {
		return provider.ResponseMetadata{}
	}
	return result.Response.ResponseMetadata
}

func responseMetadataAttrs(meta provider.ResponseMetadata) []slog.Attr {
	attrs := make([]slog.Attr, 0, 5)
	if meta.ID != "" {
		attrs = append(attrs, slog.String("ai_sdk.response.id", meta.ID))
	}
	if meta.Provider != "" {
		attrs = append(attrs, slog.String("ai_sdk.response.provider", meta.Provider))
	}
	if meta.ModelID != "" {
		attrs = append(attrs,
			slog.String("ai_sdk.response.model", meta.ModelID),
			slog.String("gen_ai.response.model", meta.ModelID),
		)
	}
	if !meta.Timestamp.IsZero() {
		attrs = append(attrs, slog.Time("ai_sdk.response.timestamp", meta.Timestamp))
	}
	return attrs
}

func finishReasonAttrs(reason provider.FinishReason) []slog.Attr {
	attrs := make([]slog.Attr, 0, 2)
	if reason.Unified != "" {
		attrs = append(attrs, slog.String("ai_sdk.finish_reason", string(reason.Unified)))
	}
	if reason.Raw != "" {
		attrs = append(attrs, slog.String("ai_sdk.finish_reason.raw", reason.Raw))
	}
	return attrs
}

func usageAttrs(usage provider.Usage) []slog.Attr {
	attrs := make([]slog.Attr, 0, 10)
	if usage.InputTokens.Total != nil {
		attrs = append(attrs,
			slog.Int("ai_sdk.usage.input_tokens.total", *usage.InputTokens.Total),
			slog.Int("gen_ai.usage.input_tokens", *usage.InputTokens.Total),
		)
	}
	if usage.InputTokens.NoCache != nil {
		attrs = append(attrs, slog.Int("ai_sdk.usage.input_tokens.no_cache", *usage.InputTokens.NoCache))
	}
	if usage.InputTokens.CacheRead != nil {
		attrs = append(attrs, slog.Int("ai_sdk.usage.input_tokens.cache_read", *usage.InputTokens.CacheRead))
	}
	if usage.InputTokens.CacheWrite != nil {
		attrs = append(attrs, slog.Int("ai_sdk.usage.input_tokens.cache_write", *usage.InputTokens.CacheWrite))
	}
	if usage.OutputTokens.Total != nil {
		attrs = append(attrs,
			slog.Int("ai_sdk.usage.output_tokens.total", *usage.OutputTokens.Total),
			slog.Int("gen_ai.usage.output_tokens", *usage.OutputTokens.Total),
		)
	}
	if usage.OutputTokens.Text != nil {
		attrs = append(attrs, slog.Int("ai_sdk.usage.output_tokens.text", *usage.OutputTokens.Text))
	}
	if usage.OutputTokens.Reasoning != nil {
		attrs = append(attrs, slog.Int("ai_sdk.usage.output_tokens.reasoning", *usage.OutputTokens.Reasoning))
	}
	return attrs
}

func warningAttrs(warnings []provider.Warning) []slog.Attr {
	attrs := []slog.Attr{slog.Int("ai_sdk.warnings.count", len(warnings))}
	if len(warnings) == 0 {
		return attrs
	}
	types := make([]string, 0, len(warnings))
	features := make([]string, 0, len(warnings))
	seenFeatures := map[string]struct{}{}
	for _, warning := range warnings {
		if warning.Type != "" {
			types = append(types, string(warning.Type))
		}
		if warning.Feature != "" {
			if _, ok := seenFeatures[warning.Feature]; !ok {
				features = append(features, warning.Feature)
				seenFeatures[warning.Feature] = struct{}{}
			}
		}
	}
	if len(types) > 0 {
		attrs = append(attrs, slog.Any("ai_sdk.warnings.types", types))
	}
	if len(features) > 0 {
		sort.Strings(features)
		attrs = append(attrs, slog.Any("ai_sdk.warnings.features", features))
	}
	return attrs
}

func errorAttrs(err error, capture CaptureOptions) []slog.Attr {
	if err == nil {
		return nil
	}
	attrs := baseErrorAttrs(err, capture)
	var apiErr *provider.APICallError
	if errors.As(err, &apiErr) {
		attrs = append(attrs, apiCallErrorAttrs(apiErr, capture)...)
	}
	return attrs
}

func baseErrorAttrs(err error, capture CaptureOptions) []slog.Attr {
	if err == nil {
		return nil
	}
	return []slog.Attr{
		slog.String("ai_sdk.error.type", errorClass(err)),
		slog.String("ai_sdk.error.type.go", errorType(err)),
		slog.String("ai_sdk.error.message", boundString(err.Error(), capture.MaxStringLen)),
	}
}

func apiCallErrorAttrs(err *provider.APICallError, capture CaptureOptions) []slog.Attr {
	if err == nil {
		return nil
	}
	attrs := []slog.Attr{
		slog.Int("ai_sdk.error.status_code", err.StatusCode),
		slog.Bool("ai_sdk.error.retryable", err.IsRetryable),
	}
	if capture.RequestBody && err.URL != "" {
		attrs = append(attrs, slog.String("ai_sdk.error.url", boundString(err.URL, capture.MaxStringLen)))
	}
	if capture.RequestBody && len(err.RequestBodyValues) > 0 {
		attrs = appendJSONAttr(attrs, "ai_sdk.request.body", err.RequestBodyValues, capture)
	}
	if capture.Headers && len(err.ResponseHeaders) > 0 {
		attrs = append(attrs, slog.Any("ai_sdk.response.headers", cloneStringSliceMap(err.ResponseHeaders)))
	}
	if capture.ResponseBody && err.ResponseBody != "" {
		attrs = append(attrs, slog.String("ai_sdk.response.body", boundString(err.ResponseBody, capture.MaxStringLen)))
	}
	return attrs
}

func apiCallPartErrorAttrs(err *provider.APICallError, capture CaptureOptions) []slog.Attr {
	if err == nil {
		return nil
	}
	attrs := baseErrorAttrs(err, capture)
	attrs = append(attrs, apiCallErrorAttrs(err, capture)...)
	return attrs
}

func errorClass(err error) string {
	if err == nil {
		return ""
	}
	var apiErr *provider.APICallError
	if errors.As(err, &apiErr) {
		return "api_call_error"
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context_deadline_exceeded"
	}
	return "unknown"
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	t := reflect.TypeOf(err)
	if t == nil {
		return "error"
	}
	return t.String()
}

func appendJSONAttr(attrs []slog.Attr, key string, value any, capture CaptureOptions) []slog.Attr {
	attr, ok := jsonAttr(key, value, capture)
	if !ok {
		return append(attrs, slog.String("ai_sdk.serialization_error", key))
	}
	return append(attrs, attr)
}

func jsonAttr(key string, value any, capture CaptureOptions) (slog.Attr, bool) {
	data, err := json.Marshal(value)
	if err != nil {
		return slog.Attr{}, false
	}
	if len(data) > capture.MaxJSONBytes {
		return truncatedJSONAttr(key, data), true
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return slog.Attr{}, false
	}
	return slog.Any(key, decoded), true
}

func truncatedJSONAttr(key string, data []byte) slog.Attr {
	attrs := []slog.Attr{
		slog.Int("bytes", len(data)),
		slog.Bool("truncated", true),
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return slog.Attr{Key: key, Value: slog.GroupValue(attrs...)}
	}
	switch value := decoded.(type) {
	case map[string]any:
		keys := make([]string, 0, len(value))
		for k := range value {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > 20 {
			keys = keys[:20]
			attrs = append(attrs, slog.Bool("keys_truncated", true))
		}
		attrs = append(attrs, slog.Any("object_keys", keys))
	case []any:
		attrs = append(attrs, slog.Int("array_length", len(value)))
	}
	return slog.Attr{Key: key, Value: slog.GroupValue(attrs...)}
}

func boundString(value string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = DefaultMaxStringLen
	}
	if len(value) <= maxLen {
		return value
	}
	const suffix = "...[truncated]"
	if maxLen <= len(suffix) {
		return value[:maxLen]
	}
	return value[:maxLen-len(suffix)] + suffix
}

func shouldCaptureGeneratedContent(capture CaptureOptions) bool {
	return capture.Outputs || capture.Reasoning || capture.ToolInputs || capture.ToolOutputs || capture.Files
}

func sanitizeMessages(messages []provider.Message, capture CaptureOptions) []provider.Message {
	out := make([]provider.Message, len(messages))
	for i, message := range messages {
		out[i] = message
		if !capture.ProviderOptions {
			out[i].ProviderOptions = nil
		}
		out[i].Content = make([]provider.ContentPart, len(message.Content))
		for j, part := range message.Content {
			out[i].Content[j] = sanitizeContentPart(part, capture)
		}
	}
	return out
}

func sanitizeContentPart(part provider.ContentPart, capture CaptureOptions) provider.ContentPart {
	if !capture.ProviderOptions {
		part.ProviderOptions = nil
	}
	switch part.Type {
	case provider.ContentPartTypeText:
		if !capture.Inputs && !capture.Outputs {
			part.Text = ""
		}
	case provider.ContentPartTypeReasoning:
		if !capture.Reasoning {
			part.Text = ""
		}
	case provider.ContentPartTypeFile, provider.ContentPartTypeReasoningFile:
		if !capture.Files {
			part.Data = nil
			part.Filename = ""
		}
	case provider.ContentPartTypeToolCall:
		if !capture.ToolInputs {
			part.Input = nil
		}
	case provider.ContentPartTypeToolResult:
		if !capture.ToolOutputs {
			part.Output = nil
		}
	}
	return part
}

func sanitizeTools(tools []provider.Tool, capture CaptureOptions) []provider.Tool {
	out := make([]provider.Tool, len(tools))
	for i, tool := range tools {
		out[i] = tool
		if !capture.ToolInputs {
			out[i].InputSchema = nil
			out[i].InputExamples = nil
			out[i].Args = nil
		}
		if !capture.ProviderOptions {
			out[i].ProviderOptions = nil
		}
	}
	return out
}

func sanitizeGenerateContent(parts []provider.GenerateContentPart, capture CaptureOptions) []provider.GenerateContentPart {
	out := make([]provider.GenerateContentPart, len(parts))
	for i, part := range parts {
		out[i] = sanitizeGenerateContentPart(part, capture)
	}
	return out
}

func sanitizeGenerateContentPart(part provider.GenerateContentPart, capture CaptureOptions) provider.GenerateContentPart {
	if !capture.ProviderMetadata {
		part.ProviderMetadata = nil
	}
	switch part.Type {
	case provider.ContentText:
		if !capture.Outputs {
			part.Text = ""
		}
	case provider.ContentReasoning:
		if !capture.Reasoning {
			part.Text = ""
		}
	case provider.ContentToolCall:
		if !capture.ToolInputs {
			part.Input = nil
		}
	case provider.ContentToolResult:
		if !capture.ToolOutputs {
			part.Result = nil
		}
	case provider.ContentFile, provider.ContentReasoningFile:
		if !capture.Files {
			part.Data = nil
			part.Filename = ""
		}
	}
	return part
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneStringSliceMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}
