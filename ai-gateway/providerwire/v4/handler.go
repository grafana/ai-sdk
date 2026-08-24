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

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

const (
	// LanguageModelPath is the relative ProviderWire language-model endpoint.
	LanguageModelPath = "/language-model"
	// HeaderSpecificationVersion carries the provider contract major version.
	HeaderSpecificationVersion = "ai-language-model-specification-version"
	// HeaderModelID carries the exact public model identifier to resolve.
	HeaderModelID = "ai-language-model-id"
	// HeaderStreaming selects unary or streaming execution.
	HeaderStreaming = "ai-language-model-streaming"
	// SpecificationVersion is the supported ProviderWire contract version.
	SpecificationVersion = "4"
)

var (
	//go:embed schema/request.json
	requestSchemaJSON []byte
	//go:embed schema/unary_success.json
	unarySuccessSchemaJSON []byte
	//go:embed schema/error.json
	errorSchemaJSON []byte
)

var canonicalInternalError = []byte(`{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`)
var canonicalInvalidRequestError = []byte(`{"error":{"message":"invalid request","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)

// Limits bounds every untrusted request, response, and model-duration resource
// used by the unary handler.
type Limits struct {
	// RequestBytes is the maximum raw request body size.
	RequestBytes int64
	// JSONDepth is the maximum object and array nesting depth.
	JSONDepth int
	// JSONTokens is the maximum number of semantic JSON values and object names.
	JSONTokens int
	// NumberBytes is the maximum byte length of one JSON number token.
	NumberBytes int
	// UnaryResponseBytes is the maximum encoded successful response size.
	UnaryResponseBytes int64
	// ErrorResponseBytes is the maximum encoded error response size.
	ErrorResponseBytes int64
	// ModelDuration is the maximum total duration of one model call.
	ModelDuration time.Duration
}

// Policy applies host-owned controls after wire mapping and before model
// resolution. The unary handler calls it only after a request is fully mapped.
type Policy interface {
	Apply(context.Context, provider.CallOptions) (provider.CallOptions, error)
}

// Config configures an immutable ProviderWire V4 unary handler.
type Config struct {
	// Resolver resolves the exact public model ID from the protocol header.
	Resolver catalog.ModelResolver
	// Policy applies optional host controls. Nil selects a no-op policy.
	Policy Policy
	// Limits bounds request processing, responses, and model execution.
	Limits Limits
}

type noOpPolicy struct{}

func (noOpPolicy) Apply(_ context.Context, options provider.CallOptions) (provider.CallOptions, error) {
	return options, nil
}

type handler struct {
	resolver           catalog.ModelResolver
	policy             Policy
	limits             Limits
	requestSchema      *schema.CompiledSchema
	unarySuccessSchema *schema.CompiledSchema
	errorSchema        *schema.CompiledSchema
	mapRequest         func([]byte) (provider.CallOptions, *requestFailure)
}

// New constructs an immutable strict ProviderWire V4 unary HTTP handler.
func New(config Config) (http.Handler, error) {
	if isNil(config.Resolver) {
		return nil, fmt.Errorf("providerwire v4: resolver is nil")
	}
	if err := validateLimits(config.Limits); err != nil {
		return nil, err
	}

	requestSchema, err := schema.CompileSchema(requestSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("providerwire v4: compiling request schema: %w", err)
	}
	unarySuccessSchema, err := schema.CompileSchema(unarySuccessSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("providerwire v4: compiling unary success schema: %w", err)
	}
	errorSchema, err := schema.CompileSchema(errorSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("providerwire v4: compiling error schema: %w", err)
	}
	if err := errorSchema.Validate(json.RawMessage(canonicalInternalError)); err != nil {
		return nil, fmt.Errorf("providerwire v4: validating canonical internal error: %w", err)
	}

	policy := config.Policy
	if isNil(policy) {
		policy = noOpPolicy{}
	}

	return &handler{
		resolver:           config.Resolver,
		policy:             policy,
		limits:             config.Limits,
		requestSchema:      requestSchema,
		unarySuccessSchema: unarySuccessSchema,
		errorSchema:        errorSchema,
		mapRequest:         mapWireRequest,
	}, nil
}

func validateLimits(limits Limits) error {
	byteLimits := []struct {
		name  string
		value int64
	}{
		{name: "request bytes", value: limits.RequestBytes},
		{name: "unary response bytes", value: limits.UnaryResponseBytes},
		{name: "error response bytes", value: limits.ErrorResponseBytes},
	}
	for _, limit := range byteLimits {
		if limit.value <= 0 {
			return fmt.Errorf("providerwire v4: %s must be positive", limit.name)
		}
		if limit.value == math.MaxInt64 {
			return fmt.Errorf("providerwire v4: %s cannot safely use limit+1", limit.name)
		}
	}
	if limits.JSONDepth <= 0 {
		return fmt.Errorf("providerwire v4: json depth must be positive")
	}
	if limits.JSONTokens <= 0 {
		return fmt.Errorf("providerwire v4: json tokens must be positive")
	}
	if limits.NumberBytes <= 0 {
		return fmt.Errorf("providerwire v4: number bytes must be positive")
	}
	if limits.NumberBytes == int(^uint(0)>>1) {
		return fmt.Errorf("providerwire v4: number bytes cannot safely use limit+1")
	}
	if limits.ModelDuration <= 0 {
		return fmt.Errorf("providerwire v4: model duration must be positive")
	}
	if int64(len(canonicalInternalError)) > limits.ErrorResponseBytes {
		return fmt.Errorf("providerwire v4: error response bytes cannot contain canonical internal error")
	}
	return nil
}

func isNil(value any) bool {
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

type requestStage string

const (
	stageEnvelope requestStage = "envelope"
	stageBody     requestStage = "body"
	stageLexical  requestStage = "lexical"
	stageSchema   requestStage = "schema"
	stageMapping  requestStage = "mapping"
)

type requestFailure struct {
	stage requestStage
	safe  safeError
}

type validatedRequest struct {
	modelID string
	body    []byte
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	validated, failure := h.validateRequest(r)
	if failure != nil {
		h.writeFailure(w, failure)
		return
	}
	options, failure := h.mapRequest(validated.body)
	if failure != nil {
		h.writeFailure(w, failure)
		return
	}
	options, err := h.applyPolicy(r.Context(), options)
	if err != nil {
		h.writeSafeError(w, safeErrorFromPolicy(err))
		return
	}
	resolved, err := h.resolveModel(r.Context(), validated.modelID)
	if err != nil {
		h.writeSafeError(w, safeErrorFromResolution(err))
		return
	}
	if !validResolvedModel(resolved) {
		h.writeSafeError(w, safeError{category: safeInternal})
		return
	}
	result, err := h.invokeModel(r.Context(), resolved.Model, options)
	if err != nil {
		h.writeSafeError(w, safeErrorFromProvider(err))
		return
	}
	if result == nil || !h.writeUnarySuccess(w, result, resolved.ID) {
		h.writeSafeError(w, safeError{category: safeInternal})
	}
}

func (h *handler) validateRequest(r *http.Request) (validatedRequest, *requestFailure) {
	modelID, failure := validateEnvelope(r)
	if failure != nil {
		return validatedRequest{}, failure
	}

	body, failure := h.readBody(r.Body)
	if failure != nil {
		return validatedRequest{}, failure
	}
	if !scanJSON(body, h.limits.JSONDepth, h.limits.JSONTokens, h.limits.NumberBytes) {
		return validatedRequest{}, &requestFailure{stage: stageLexical}
	}
	if err := h.requestSchema.Validate(json.RawMessage(body)); err != nil {
		return validatedRequest{}, &requestFailure{stage: stageSchema}
	}
	return validatedRequest{modelID: modelID, body: body}, nil
}

func validateEnvelope(r *http.Request) (string, *requestFailure) {
	if r.Method != http.MethodPost || r.URL == nil || r.URL.Path != LanguageModelPath || r.URL.EscapedPath() != LanguageModelPath {
		return "", &requestFailure{stage: stageEnvelope}
	}
	contentType, ok := singleHeaderValue(r.Header, "Content-Type")
	if !ok {
		return "", &requestFailure{stage: stageEnvelope}
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return "", &requestFailure{stage: stageEnvelope}
	}

	specification, ok := singleHeaderValue(r.Header, HeaderSpecificationVersion)
	if !ok || specification != SpecificationVersion {
		return "", &requestFailure{stage: stageEnvelope}
	}
	modelID, ok := singleHeaderValue(r.Header, HeaderModelID)
	if !ok || modelID == "" {
		return "", &requestFailure{stage: stageEnvelope}
	}
	streaming, ok := singleHeaderValue(r.Header, HeaderStreaming)
	if !ok || streaming != "false" {
		return "", &requestFailure{stage: stageEnvelope}
	}
	return modelID, nil
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
		return nil, &requestFailure{stage: stageBody}
	}
	data, readErr := io.ReadAll(io.LimitReader(body, h.limits.RequestBytes+1))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil {
		return nil, &requestFailure{stage: stageBody, safe: safeError{category: safeInternal}}
	}
	if int64(len(data)) > h.limits.RequestBytes {
		return nil, &requestFailure{stage: stageBody}
	}
	return data, nil
}

func (h *handler) writeFailure(w http.ResponseWriter, failure *requestFailure) {
	value := failure.safe
	if value.category == 0 {
		value.category = safeInvalidRequest
	}
	h.writeSafeError(w, value)
}

func invalidMappingFailure() *requestFailure {
	return &requestFailure{stage: stageMapping, safe: safeError{category: safeInvalidRequest}}
}

func unsupportedMappingFailure(capability unsupportedCapability) *requestFailure {
	return &requestFailure{
		stage: stageMapping,
		safe:  safeError{category: safeInvalidRequest, capability: capability},
	}
}

var errRuntimeInternal = errors.New("providerwire v4: runtime internal failure")

func (h *handler) applyPolicy(ctx context.Context, options provider.CallOptions) (result provider.CallOptions, err error) {
	defer func() {
		if recover() != nil {
			result = provider.CallOptions{}
			err = errRuntimeInternal
		}
	}()
	return h.policy.Apply(ctx, options)
}

func (h *handler) resolveModel(ctx context.Context, modelID string) (resolved catalog.ResolvedModel, err error) {
	defer func() {
		if recover() != nil {
			resolved = catalog.ResolvedModel{}
			err = errRuntimeInternal
		}
	}()
	return h.resolver.ResolveModel(ctx, modelID)
}

func validResolvedModel(resolved catalog.ResolvedModel) (valid bool) {
	defer func() {
		if recover() != nil {
			valid = false
		}
	}()
	return resolved.ID != "" && !isNil(resolved.Model) && resolved.Model.SpecificationVersion() == "v4"
}

type modelOutcome struct {
	result *provider.GenerateResult
	err    error
}

var errModelInternal = errors.New("providerwire v4: model internal failure")

func (h *handler) invokeModel(ctx context.Context, model provider.LanguageModel, options provider.CallOptions) (*provider.GenerateResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	modelContext, cancel := context.WithTimeout(ctx, h.limits.ModelDuration)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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
		outcome.result, outcome.err = model.DoGenerate(modelContext, options)
	}()

	select {
	case outcome := <-outcomes:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := modelContext.Err(); err != nil {
			return nil, err
		}
		return outcome.result, outcome.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-modelContext.Done():
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, context.DeadlineExceeded
	}
}
