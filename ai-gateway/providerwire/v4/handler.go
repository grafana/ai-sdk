package v4

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
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

type handler struct {
	runtime       *execution.Runtime
	requestSchema *schema.CompiledSchema
}

// New constructs an immutable strict ProviderWire V4 HTTP handler over runtime,
// which selects and invokes the model.
func New(runtime *execution.Runtime) (http.Handler, error) {
	if runtime == nil {
		return nil, fmt.Errorf("providerwire v4: runtime is nil")
	}
	requestSchema, err := schema.CompileSchema(requestSchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("providerwire v4: compiling request schema: %w", err)
	}
	return &handler{runtime: runtime, requestSchema: requestSchema}, nil
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
	call, err := h.runtime.Call(r.Context(), validated.modelID, options)
	if err != nil {
		h.writeSafeError(w, safeErrorFromResolution(err), nil)
		return
	}
	defer call.Close()
	if validated.mode == executionStreaming {
		h.serveStream(w, call, options, history)
		return
	}
	result, err := call.Generate()
	if err != nil {
		view := call.View(err)
		h.writeSafeError(w, safeErrorFromProvider(err), view.Metadata())
		return
	}
	view := call.View(nil)
	if result == nil || !h.writeUnarySuccess(w, result, unaryMappingContext{history: history, execution: view}) {
		h.writeSafeError(w, safeError{category: safeInternal}, view.Metadata())
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
	data, readErr := io.ReadAll(body)
	closeErr := body.Close()
	var tooLarge *http.MaxBytesError
	if errors.As(readErr, &tooLarge) {
		return nil, &requestFailure{}
	}
	if readErr != nil || closeErr != nil {
		return nil, &requestFailure{safe: safeError{category: safeInternal}}
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
