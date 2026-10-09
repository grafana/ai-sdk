package v4

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

const (
	// LanguageModelPath is the relative ProviderWire language-model endpoint.
	LanguageModelPath = "/language-model"
	// HeaderSpecificationVersion carries the provider contract major version.
	HeaderSpecificationVersion = "ai-language-model-specification-version"
	// HeaderModelID carries the exact model selector for the authenticated policy.
	HeaderModelID = "ai-language-model-id"
	// HeaderStreaming selects unary or streaming execution.
	HeaderStreaming = "ai-language-model-streaming"
	// SpecificationVersion is the supported ProviderWire contract version.
	SpecificationVersion = "4"
)

var (
	//go:embed schema/request.json
	requestSchemaJSON []byte
)

// Limits bounds every untrusted request, response, and model-duration resource
// used by the unary and streaming handler.
type Limits struct {
	// RequestBytes is the maximum raw request body size.
	RequestBytes int64
	// UnaryResponseBytes is the maximum encoded successful response size.
	UnaryResponseBytes int64
	// StreamParts is the maximum number of provider stream values received.
	StreamParts int
	// StreamFrameBytes is the maximum complete SSE frame size.
	StreamFrameBytes int64
	// ModelDuration bounds selection and the complete logical model call.
	ModelDuration time.Duration
	// StreamIdleDuration is the maximum time between represented provider parts.
	StreamIdleDuration time.Duration
	// StreamDrainDuration bounds asynchronous post-terminal channel draining.
	StreamDrainDuration time.Duration
}

// Config configures an immutable ProviderWire V4 language-model handler.
type Config struct {
	Selector RequestSelector
	// Limits bounds request processing, responses, and model execution.
	Limits Limits
}

type handler struct {
	selector      RequestSelector
	limits        Limits
	requestSchema *schema.CompiledSchema
}

// New constructs an immutable strict ProviderWire V4 HTTP handler.
func New(config Config) (http.Handler, error) {
	if config.Selector == nil {
		return nil, fmt.Errorf("providerwire v4: selector is nil")
	}
	if err := validateLimits(config.Limits); err != nil {
		return nil, err
	}

	requestSchema, err := schema.CompileSchema(requestSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("providerwire v4: compiling request schema: %w", err)
	}
	return &handler{
		selector:      config.Selector,
		limits:        config.Limits,
		requestSchema: requestSchema,
	}, nil
}

func validateLimits(limits Limits) error {
	byteLimits := []struct {
		name          string
		value         int64
		requiresExtra bool
	}{
		{name: "request bytes", value: limits.RequestBytes, requiresExtra: true},
		{name: "unary response bytes", value: limits.UnaryResponseBytes, requiresExtra: true},
		{name: "stream frame bytes", value: limits.StreamFrameBytes},
	}
	for _, limit := range byteLimits {
		if limit.value <= 0 {
			return fmt.Errorf("providerwire v4: %s must be positive", limit.name)
		}
		if limit.requiresExtra && limit.value == math.MaxInt64 {
			return fmt.Errorf("providerwire v4: %s cannot safely use limit+1", limit.name)
		}
	}
	if limits.StreamParts <= 0 {
		return fmt.Errorf("providerwire v4: stream parts must be positive")
	}
	if limits.StreamParts == int(^uint(0)>>1) {
		return fmt.Errorf("providerwire v4: stream parts cannot safely use limit+1")
	}
	if limits.ModelDuration <= 0 {
		return fmt.Errorf("providerwire v4: model duration must be positive")
	}
	if limits.StreamIdleDuration <= 0 {
		return fmt.Errorf("providerwire v4: stream idle duration must be positive")
	}
	if limits.StreamDrainDuration <= 0 {
		return fmt.Errorf("providerwire v4: stream drain duration must be positive")
	}
	for name, frame := range map[string][]byte{
		"empty stream start":        canonicalEmptyStartFrame,
		"rate-limit stream error":   streamErrorFrameForSafeError(safeError{category: safeRateLimit}),
		"overload stream error":     streamErrorFrameForSafeError(safeError{category: safeOverload}),
		"dependency stream error":   streamErrorFrameForSafeError(safeError{category: safeFailedDependency}),
		"upstream stream error":     streamErrorFrameForSafeError(safeError{category: safeUpstream}),
		"internal stream error":     streamErrorFrameForSafeError(safeError{category: safeInternal}),
		"timeout stream error":      streamErrorFrameForSafeError(safeError{category: safeTimeout}),
		"cancellation stream error": streamErrorFrameForSafeError(safeError{category: safeCancellation}),
	} {
		if int64(len(frame)) > limits.StreamFrameBytes {
			return fmt.Errorf("providerwire v4: stream frame bytes cannot contain canonical %s", name)
		}
	}
	return nil
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

type requestFailure struct {
	safe safeError
}

type executionMode uint8

const (
	executionUnary executionMode = iota + 1
	executionStreaming
)

type validatedRequest struct {
	modelID string
	mode    executionMode
	body    []byte
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	validated, failure := h.validateRequest(r)
	if failure != nil {
		h.writeFailure(w, failure)
		return
	}
	var wire wireRequest
	if err := json.Unmarshal(validated.body, &wire); err != nil {
		h.writeFailure(w, invalidMappingFailure())
		return
	}
	gateway := wire.ProviderOptions["gateway"]
	delete(wire.ProviderOptions, "gateway")
	options, failure := mapRequest(wire)
	if failure != nil {
		h.writeFailure(w, failure)
		return
	}
	history, failure := unresolvedProviderCalls(options.Prompt)
	if failure != nil {
		h.writeFailure(w, failure)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.limits.ModelDuration)
	defer cancel()
	resolved, err := h.selectModel(ctx, validated.modelID, options, gateway)
	if err != nil {
		h.writeSafeError(w, safeErrorFromResolution(err), nil)
		return
	}
	request := newExecutionRequest(validated.modelID, resolved, h.limits.UnaryResponseBytes)
	capture := &attemptCapture{sealed: resolved.configured == nil}
	if resolved.configured != nil {
		ctx = fallback.WithAttemptObserver(ctx, capture.observe)
	}
	defer capture.discard()
	if validated.mode == executionStreaming {
		h.serveStream(w, r.Context(), ctx, resolved.Model, options, history, request, capture)
		return
	}
	result, err := h.invokeModel(ctx, resolved.Model, options, capture)
	if err != nil {
		value := safeErrorFromProvider(err)
		view := request.snapshot(capture, err)
		h.writeSafeError(w, value, view.metadata())
		return
	}
	view := request.snapshot(capture, nil)
	if result == nil || !h.writeUnarySuccess(w, result, unaryMappingContext{history: history, overview: view.overview}) {
		h.writeSafeError(w, safeError{category: safeInternal}, view.metadata())
	}
}

func (h *handler) validateRequest(r *http.Request) (validatedRequest, *requestFailure) {
	modelID, mode, failure := validateEnvelope(r)
	if failure != nil {
		return validatedRequest{}, failure
	}

	body, failure := h.readBody(r.Body)
	if failure != nil {
		return validatedRequest{}, failure
	}
	if !utf8.Valid(body) {
		return validatedRequest{}, &requestFailure{}
	}
	if err := h.requestSchema.Validate(json.RawMessage(body)); err != nil {
		return validatedRequest{}, &requestFailure{}
	}
	return validatedRequest{modelID: modelID, mode: mode, body: body}, nil
}

func validateEnvelope(r *http.Request) (string, executionMode, *requestFailure) {
	if r.Method != http.MethodPost || r.URL == nil || r.URL.Path != LanguageModelPath || r.URL.EscapedPath() != LanguageModelPath {
		return "", 0, &requestFailure{}
	}
	contentType, ok := singleHeaderValue(r.Header, "Content-Type")
	if !ok {
		return "", 0, &requestFailure{}
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return "", 0, &requestFailure{}
	}

	specification, ok := singleHeaderValue(r.Header, HeaderSpecificationVersion)
	if !ok || specification != SpecificationVersion {
		return "", 0, &requestFailure{}
	}
	modelID, ok := singleHeaderValue(r.Header, HeaderModelID)
	if !ok || modelID == "" {
		return "", 0, &requestFailure{}
	}
	streaming, ok := singleHeaderValue(r.Header, HeaderStreaming)
	if !ok {
		return "", 0, &requestFailure{}
	}
	switch streaming {
	case "false":
		return modelID, executionUnary, nil
	case "true":
		return modelID, executionStreaming, nil
	default:
		return "", 0, &requestFailure{}
	}
}

func singleHeaderValue(headers http.Header, name string) (string, bool) {
	var values []string
	for candidate, candidateValues := range headers {
		if strings.EqualFold(candidate, name) {
			values = append(values, candidateValues...)
		}
	}
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func (h *handler) readBody(body io.ReadCloser) ([]byte, *requestFailure) {
	if body == nil {
		return nil, &requestFailure{}
	}
	data, readErr := io.ReadAll(io.LimitReader(body, h.limits.RequestBytes+1))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil {
		return nil, &requestFailure{safe: safeError{category: safeInternal}}
	}
	if int64(len(data)) > h.limits.RequestBytes {
		return nil, &requestFailure{}
	}
	return data, nil
}

func (h *handler) writeFailure(w http.ResponseWriter, failure *requestFailure) {
	value := failure.safe
	if value.category == 0 {
		value.category = safeInvalidRequest
	}
	h.writeSafeError(w, value, nil)
}

func invalidMappingFailure() *requestFailure {
	return &requestFailure{safe: safeError{category: safeInvalidRequest}}
}

func rejectedMappingFailure(reason requestFailureReason) *requestFailure {
	return &requestFailure{safe: safeError{category: safeInvalidRequest, reason: reason}}
}

var errRuntimeInternal = errors.New("providerwire v4: runtime internal failure")

func validSelection(resolved Selection) (valid bool) {
	defer func() {
		if recover() != nil {
			valid = false
		}
	}()
	return resolved.ID != "" && utf8.ValidString(resolved.ID) && !isNilInterface(resolved.Model) && resolved.Model.SpecificationVersion() == "v4"
}

type modelOutcome struct {
	result *provider.GenerateResult
	err    error
}

var errModelInternal = errors.New("providerwire v4: model internal failure")

func (h *handler) invokeModel(ctx context.Context, model provider.LanguageModel, options provider.CallOptions, capture *attemptCapture) (*provider.GenerateResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	modelContext, cancel := context.WithCancel(ctx)
	defer cancel()

	outcomes := make(chan modelOutcome, 1)
	go func() {
		outcome := modelOutcome{}
		defer func() {
			if recover() != nil {
				outcome = modelOutcome{err: errModelInternal}
			}
			if outcome.result == nil && outcome.err == nil {
				outcome.err = errModelInternal
			}
			outcomes <- outcome
		}()
		capture.enter()
		outcome.result, outcome.err = model.DoGenerate(modelContext, options)
	}()

	rejectOutcome := func(err error) (*provider.GenerateResult, error) {
		capture.discard()
		return nil, err
	}
	select {
	case outcome := <-outcomes:
		if err := ctx.Err(); err != nil {
			return rejectOutcome(err)
		}
		return outcome.result, outcome.err
	case <-ctx.Done():
		return rejectOutcome(ctx.Err())
	}
}
