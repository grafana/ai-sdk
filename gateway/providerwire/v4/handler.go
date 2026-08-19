package providerwirev4

import (
	"context"
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

	"github.com/grafana/ai-sdk/provider"
)

const (
	// PathLanguageModel is the exact path served by the V4 handler.
	PathLanguageModel = "/language-model"
	// HeaderModelID carries the public model identifier.
	HeaderModelID = "ai-language-model-id"
	// HeaderStreaming selects unary or streaming behavior.
	HeaderStreaming = "ai-language-model-streaming"
	// HeaderSpecVersion carries the LanguageModel specification version.
	HeaderSpecVersion = "ai-language-model-specification-version"
	// SpecVersionV4 is the supported specification version.
	SpecVersionV4 = "4"
	// MIMEJSON is the ProviderWire unary media type.
	MIMEJSON = "application/json"

	// DefaultTotalTimeout bounds resolution and unary generation.
	DefaultTotalTimeout = 120 * time.Second
	// DefaultMaxRequestBodyBytes is the raw request body limit.
	DefaultMaxRequestBodyBytes int64 = 8 << 20
	// DefaultMaxInlineFileBytes is the aggregate decoded inline-file limit.
	DefaultMaxInlineFileBytes int64 = 8 << 20
	// DefaultMaxResponseBodyBytes is the unary success body limit.
	DefaultMaxResponseBodyBytes int64 = 8 << 20
	// DefaultMaxErrorBodyBytes is the safe error body limit.
	DefaultMaxErrorBodyBytes int64 = 16 << 10
)

var (
	// ErrTotalTimeout identifies expiration of the V4 total operation timeout.
	ErrTotalTimeout = errors.New("providerwirev4: total timeout")
)

const fallbackErrorJSON = `{"error":{"type":"internal_server_error","message":"internal server error","statusCode":500,"isRetryable":true}}`

// ModelResolver selects a model after request validation and policy acceptance.
type ModelResolver interface {
	ResolveLanguageModel(r *http.Request, modelID string) (provider.LanguageModel, error)
}

// ModelResolverFunc adapts a function to ModelResolver.
type ModelResolverFunc func(r *http.Request, modelID string) (provider.LanguageModel, error)

// ResolveLanguageModel implements ModelResolver.
func (f ModelResolverFunc) ResolveLanguageModel(r *http.Request, modelID string) (provider.LanguageModel, error) {
	return f(r, modelID)
}

// Option configures a Handler.
type Option func(*handlerOptions) error

type handlerOptions struct {
	totalTimeout         time.Duration
	maxRequestBodyBytes  int64
	maxInlineFileBytes   int64
	maxResponseBodyBytes int64
	maxErrorBodyBytes    int64
}

// WithTotalTimeout sets the resolution and generation timeout.
func WithTotalTimeout(timeout time.Duration) Option {
	return func(options *handlerOptions) error {
		if timeout <= 0 {
			return errors.New("providerwirev4: total timeout must be positive")
		}
		options.totalTimeout = timeout
		return nil
	}
}

// WithMaxRequestBodyBytes sets the raw request body limit.
func WithMaxRequestBodyBytes(limit int64) Option {
	return positiveLimitOption("request body", limit, func(options *handlerOptions, value int64) { options.maxRequestBodyBytes = value })
}

// WithMaxInlineFileBytes sets the aggregate decoded inline-file limit.
func WithMaxInlineFileBytes(limit int64) Option {
	return positiveLimitOption("inline file", limit, func(options *handlerOptions, value int64) { options.maxInlineFileBytes = value })
}

// WithMaxResponseBodyBytes sets the unary success body limit.
func WithMaxResponseBodyBytes(limit int64) Option {
	return positiveLimitOption("response body", limit, func(options *handlerOptions, value int64) { options.maxResponseBodyBytes = value })
}

// WithMaxErrorBodyBytes sets the safe error body limit.
func WithMaxErrorBodyBytes(limit int64) Option {
	return positiveLimitOption("error body", limit, func(options *handlerOptions, value int64) { options.maxErrorBodyBytes = value })
}

func positiveLimitOption(name string, limit int64, assign func(*handlerOptions, int64)) Option {
	return func(options *handlerOptions) error {
		if limit <= 0 {
			return fmt.Errorf("providerwirev4: maximum %s bytes must be positive", name)
		}
		assign(options, limit)
		return nil
	}
}

// Handler serves strict unary ProviderWire V4 calls.
type Handler struct {
	resolver             ModelResolver
	registry             *schemaRegistry
	totalTimeout         time.Duration
	maxRequestBodyBytes  int64
	maxInlineFileBytes   int64
	maxResponseBodyBytes int64
	maxErrorBodyBytes    int64
}

// NewHandler constructs a strict unary ProviderWire V4 handler.
func NewHandler(resolver ModelResolver, options ...Option) (*Handler, error) {
	if isNilInterface(resolver) {
		return nil, errors.New("providerwirev4: nil model resolver")
	}
	configured := handlerOptions{
		totalTimeout:         DefaultTotalTimeout,
		maxRequestBodyBytes:  DefaultMaxRequestBodyBytes,
		maxInlineFileBytes:   DefaultMaxInlineFileBytes,
		maxResponseBodyBytes: DefaultMaxResponseBodyBytes,
		maxErrorBodyBytes:    DefaultMaxErrorBodyBytes,
	}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("providerwirev4: nil option")
		}
		if err := option(&configured); err != nil {
			return nil, err
		}
	}
	if configured.maxErrorBodyBytes < int64(len(fallbackErrorJSON)) {
		return nil, fmt.Errorf("providerwirev4: maximum error body bytes must be at least %d", len(fallbackErrorJSON))
	}
	registry, err := loadEmbeddedContractRegistry()
	if err != nil {
		return nil, err
	}
	if err := registry.validateErrorEnvelope([]byte(fallbackErrorJSON), http.StatusInternalServerError); err != nil {
		return nil, fmt.Errorf("providerwirev4: validating fallback error: %w", err)
	}
	return &Handler{
		resolver:             resolver,
		registry:             registry,
		totalTimeout:         configured.totalTimeout,
		maxRequestBodyBytes:  configured.maxRequestBodyBytes,
		maxInlineFileBytes:   configured.maxInlineFileBytes,
		maxResponseBodyBytes: configured.maxResponseBodyBytes,
		maxErrorBodyBytes:    configured.maxErrorBodyBytes,
	}, nil
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// ServeHTTP validates, resolves, and invokes one unary model call.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	modelID, failure := validateUnaryEnvelope(r)
	if failure != nil {
		h.writeFailure(w, *failure)
		return
	}
	body, failure := h.readRequestBody(r)
	if failure != nil {
		h.writeFailure(w, *failure)
		return
	}
	if _, err := validateStrictJSON(body); err != nil {
		h.writeFailure(w, requestFailure(http.StatusBadRequest, "invalid JSON syntax"))
		return
	}
	if err := h.registry.validateSyntaxChecked("request", body); err != nil {
		message := "request does not match the ProviderWire V4 schema"
		if path := safeValidationPath(err); path != "" {
			message += " at " + path
		}
		h.writeFailure(w, requestFailure(http.StatusBadRequest, message))
		return
	}
	request, err := decodeWireRequest(body)
	if err != nil {
		h.writeFailure(w, requestFailure(http.StatusBadRequest, "request could not be decoded"))
		return
	}
	if err := applyRequestPolicy(&request); err != nil {
		h.writeFailure(w, requestFailure(http.StatusBadRequest, "request violates this service's policy"))
		return
	}
	adapter := requestAdapter{maxInlineBytes: h.maxInlineFileBytes}
	callOptions, err := adapter.adaptPolicyChecked(request)
	if err != nil {
		h.writeFailure(w, requestFailure(http.StatusBadRequest, "request cannot be represented by this service"))
		return
	}

	ctx, cancel := context.WithTimeoutCause(r.Context(), h.totalTimeout, ErrTotalTimeout)
	defer cancel()
	operationResult := make(chan unaryOperationResult, 1)
	go h.runUnaryOperation(ctx, r.WithContext(ctx), modelID, callOptions, operationResult)

	var result *provider.GenerateResult
	select {
	case <-ctx.Done():
		h.writeFailure(w, normalizeOperationError(ctx, ctx.Err(), operationProvider, modelID))
		return
	case outcome := <-operationResult:
		if ctx.Err() != nil {
			h.writeFailure(w, normalizeOperationError(ctx, ctx.Err(), outcome.stage, modelID))
			return
		}
		if outcome.panicked {
			h.writeFailure(w, internalFailure())
			return
		}
		if outcome.err != nil {
			h.writeFailure(w, normalizeOperationError(ctx, outcome.err, outcome.stage, modelID))
			return
		}
		if outcome.nilModel {
			h.writeFailure(w, internalFailure())
			return
		}
		result = outcome.result
	}
	encoded, err := h.prepareGenerateResponse(result)
	if err != nil {
		h.writeFailure(w, internalFailure())
		return
	}
	h.commitGenerateResponse(w, ctx, modelID, encoded)
}

func (h *Handler) prepareGenerateResponse(result *provider.GenerateResult) ([]byte, error) {
	if estimate, err := estimateGenerateResultUpperBound(result); err != nil || estimate > h.maxResponseBodyBytes {
		return nil, errors.New("providerwirev4: response exceeds configured limit")
	}
	projected, err := projectGenerateResult(result)
	if err != nil {
		return nil, fmt.Errorf("providerwirev4: projecting response: %w", err)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		return nil, fmt.Errorf("providerwirev4: marshaling response: %w", err)
	}
	if int64(len(encoded)) > h.maxResponseBodyBytes {
		return nil, errors.New("providerwirev4: encoded response exceeds configured limit")
	}
	if err := h.registry.validate("generate-result", encoded); err != nil {
		return nil, fmt.Errorf("providerwirev4: validating response: %w", err)
	}
	return encoded, nil
}

func (h *Handler) commitGenerateResponse(w http.ResponseWriter, ctx context.Context, modelID string, encoded []byte) {
	if err := ctx.Err(); err != nil {
		h.writeFailure(w, normalizeOperationError(ctx, err, operationProvider, modelID))
		return
	}
	_ = writePreparedResponse(w, http.StatusOK, encoded)
}

type unaryOperationResult struct {
	result   *provider.GenerateResult
	err      error
	stage    operationStage
	nilModel bool
	panicked bool
}

func (h *Handler) runUnaryOperation(ctx context.Context, request *http.Request, modelID string, callOptions provider.CallOptions, result chan<- unaryOperationResult) {
	outcome := unaryOperationResult{stage: operationResolver}
	defer func() {
		if recover() != nil {
			outcome = unaryOperationResult{panicked: true}
		}
		result <- outcome
	}()
	model, err := h.resolver.ResolveLanguageModel(request, modelID)
	if err != nil {
		outcome.err = err
		return
	}
	if isNilInterface(model) {
		outcome.nilModel = true
		return
	}
	outcome.stage = operationProvider
	if ctx.Err() != nil {
		outcome.err = ctx.Err()
		return
	}
	outcome.result, outcome.err = model.DoGenerate(ctx, callOptions)
}

func validateUnaryEnvelope(r *http.Request) (string, *safeFailure) {
	if r.Method != http.MethodPost {
		failure := requestFailure(http.StatusMethodNotAllowed, "method must be POST")
		return "", &failure
	}
	if r.URL == nil || r.URL.Path != PathLanguageModel {
		failure := requestFailure(http.StatusNotFound, "path must be /language-model")
		return "", &failure
	}
	modelID := r.Header.Get(HeaderModelID)
	if modelID == "" || strings.TrimSpace(modelID) != modelID {
		failure := requestFailure(http.StatusBadRequest, "model id is required and must be unpadded")
		return "", &failure
	}
	if r.Header.Get(HeaderSpecVersion) != SpecVersionV4 {
		failure := requestFailure(http.StatusBadRequest, "specification version must be 4")
		return "", &failure
	}
	switch r.Header.Get(HeaderStreaming) {
	case "false":
	case "true":
		failure := requestFailure(http.StatusBadRequest, "streaming is not supported")
		return "", &failure
	default:
		failure := requestFailure(http.StatusBadRequest, "streaming selection must be false")
		return "", &failure
	}
	contentTypeValues, present := r.Header[http.CanonicalHeaderKey("Content-Type")]
	if !present || len(contentTypeValues) == 0 {
		failure := requestFailure(http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return "", &failure
	}
	contentType, _, err := mime.ParseMediaType(strings.Join(contentTypeValues, ","))
	if err != nil || contentType != MIMEJSON {
		failure := requestFailure(http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return "", &failure
	}
	if acceptValues, present := r.Header[http.CanonicalHeaderKey("Accept")]; present {
		compatible, valid := acceptsRepresentation(strings.Join(acceptValues, ","), MIMEJSON)
		if !valid || !compatible {
			failure := requestFailure(http.StatusNotAcceptable, "Accept does not permit application/json")
			return "", &failure
		}
	}
	return modelID, nil
}

func (h *Handler) readRequestBody(r *http.Request) ([]byte, *safeFailure) {
	defer func() { _ = r.Body.Close() }()
	limit := h.maxRequestBodyBytes
	readLimit := limit
	if readLimit < math.MaxInt64 {
		readLimit++
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, readLimit))
	if err != nil {
		if r.Context().Err() != nil {
			failure := canceledFailure()
			return nil, &failure
		}
		failure := requestFailure(http.StatusBadRequest, "request body could not be read")
		return nil, &failure
	}
	if int64(len(body)) > limit {
		failure := requestFailure(http.StatusRequestEntityTooLarge, "request body is too large")
		return nil, &failure
	}
	return body, nil
}

func (h *Handler) writeFailure(w http.ResponseWriter, failure safeFailure) {
	encoded, err := json.Marshal(failure.envelope())
	if err != nil || int64(len(encoded)) > h.maxErrorBodyBytes || h.registry.validateErrorEnvelope(encoded, failure.status) != nil {
		encoded = []byte(fallbackErrorJSON)
		failure.status = http.StatusInternalServerError
	}
	_ = writePreparedResponse(w, failure.status, encoded)
}

func writePreparedResponse(w http.ResponseWriter, status int, body []byte) error {
	w.Header().Set("Content-Type", MIMEJSON)
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("providerwirev4: writing prepared response: %w", err)
	}
	return nil
}

type operationStage int

const (
	operationResolver operationStage = iota
	operationProvider
)

type safeErrorType string

const (
	errorAuthentication safeErrorType = "authentication_error"
	errorInvalidRequest safeErrorType = "invalid_request_error"
	errorRateLimit      safeErrorType = "rate_limit_exceeded"
	errorModelNotFound  safeErrorType = "model_not_found"
	errorInternal       safeErrorType = "internal_server_error"
	errorDependency     safeErrorType = "failed_dependency"
	errorForbidden      safeErrorType = "forbidden"
)

type safeFailure struct {
	status    int
	typeName  safeErrorType
	message   string
	retryable bool
	param     any
}

type safeErrorEnvelope struct {
	Error safeError `json:"error"`
}

type safeError struct {
	Type        safeErrorType `json:"type"`
	Message     string        `json:"message"`
	StatusCode  int           `json:"statusCode"`
	IsRetryable bool          `json:"isRetryable"`
	Param       any           `json:"param,omitempty"`
}

type modelParam struct {
	ModelID string `json:"modelId"`
}

func (failure safeFailure) envelope() safeErrorEnvelope {
	return safeErrorEnvelope{Error: safeError{Type: failure.typeName, Message: failure.message, StatusCode: failure.status, IsRetryable: failure.retryable, Param: failure.param}}
}

func requestFailure(status int, message string) safeFailure {
	return safeFailure{status: status, typeName: errorInvalidRequest, message: message}
}

func internalFailure() safeFailure {
	return safeFailure{status: http.StatusInternalServerError, typeName: errorInternal, message: "internal server error", retryable: true}
}

func canceledFailure() safeFailure {
	return safeFailure{status: 499, typeName: errorDependency, message: "request canceled"}
}

func normalizeOperationError(ctx context.Context, err error, stage operationStage, modelID string) safeFailure {
	cause := context.Cause(ctx)
	if errors.Is(cause, ErrTotalTimeout) {
		return safeFailure{status: http.StatusGatewayTimeout, typeName: errorDependency, message: "request timed out", retryable: true}
	}
	if ctx.Err() != nil {
		return canceledFailure()
	}
	var apiError *provider.APICallError
	if !errors.As(err, &apiError) || apiError == nil || apiError.StatusCode < 300 || apiError.StatusCode > 599 || apiError.StatusCode == http.StatusNotModified {
		if stage == operationProvider && !errors.As(err, &apiError) {
			return safeFailure{status: http.StatusFailedDependency, typeName: errorDependency, message: "provider request failed", retryable: true}
		}
		return internalFailure()
	}
	status := apiError.StatusCode
	if stage == operationResolver {
		switch status {
		case http.StatusUnauthorized:
			return safeFailure{status: status, typeName: errorAuthentication, message: "authentication failed", retryable: apiError.IsRetryable}
		case http.StatusForbidden:
			return safeFailure{status: status, typeName: errorForbidden, message: "request forbidden", retryable: apiError.IsRetryable}
		case http.StatusNotFound:
			return safeFailure{status: status, typeName: errorModelNotFound, message: "model not found", retryable: apiError.IsRetryable, param: modelParam{ModelID: modelID}}
		}
	}
	if status == http.StatusTooManyRequests {
		return safeFailure{status: status, typeName: errorRateLimit, message: "rate limit exceeded", retryable: apiError.IsRetryable}
	}
	if status >= 400 && status < 500 {
		return safeFailure{status: status, typeName: errorInvalidRequest, message: "provider rejected the request", retryable: apiError.IsRetryable}
	}
	if status >= 500 {
		return safeFailure{status: status, typeName: errorDependency, message: "provider request failed", retryable: apiError.IsRetryable}
	}
	return internalFailure()
}
