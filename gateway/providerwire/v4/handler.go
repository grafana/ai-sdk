package providerwirev4

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/gateway/runtime"
	"github.com/grafana/ai-sdk/provider"
)

const (
	// DefaultMaxRequestBodyBytes is the default bounded request read size.
	DefaultMaxRequestBodyBytes int64 = 8 << 20
	// DefaultMaxUnaryResponseBytes is the default encoded unary commitment limit.
	DefaultMaxUnaryResponseBytes int64 = 16 << 20
	// DefaultMaxSSEEventBytes is the default complete framed event limit.
	DefaultMaxSSEEventBytes int64 = 8 << 20
	// DefaultIdleTimeout is the default maximum wait between runtime parts.
	DefaultIdleTimeout = 60 * time.Second
)

var (
	// ErrIdleTimeout identifies provider inactivity while waiting for a part.
	ErrIdleTimeout = errors.New("providerwirev4: stream idle timeout")
)

// MetadataExtractor returns metadata established by host authentication.
type MetadataExtractor func(*http.Request) (runtime.CallMetadata, error)

// RequestIDGenerator returns a non-empty gateway request ID.
type RequestIDGenerator func() (string, error)

// Option configures a Handler.
type Option func(*handlerOptions) error

type handlerOptions struct {
	maxRequestBodyBytes   int64
	maxUnaryResponseBytes int64
	maxSSEEventBytes      int64
	idleTimeout           time.Duration
	metadataExtractor     MetadataExtractor
	requestIDGenerator    RequestIDGenerator
}

// WithMaxRequestBodyBytes sets the bounded request read size.
func WithMaxRequestBodyBytes(limit int64) Option {
	return positiveByteOption("request body", limit, func(options *handlerOptions, value int64) { options.maxRequestBodyBytes = value })
}

// WithMaxUnaryResponseBytes sets the encoded unary commitment limit.
func WithMaxUnaryResponseBytes(limit int64) Option {
	return positiveByteOption("unary response", limit, func(options *handlerOptions, value int64) { options.maxUnaryResponseBytes = value })
}

// WithMaxSSEEventBytes sets the complete framed event commitment limit.
func WithMaxSSEEventBytes(limit int64) Option {
	return positiveByteOption("SSE event", limit, func(options *handlerOptions, value int64) { options.maxSSEEventBytes = value })
}

func positiveByteOption(name string, limit int64, set func(*handlerOptions, int64)) Option {
	return func(options *handlerOptions) error {
		if limit <= 0 {
			return fmt.Errorf("providerwirev4: maximum %s bytes must be positive", name)
		}
		set(options, limit)
		return nil
	}
}

// WithIdleTimeout sets the provider-part idle timeout.
func WithIdleTimeout(timeout time.Duration) Option {
	return func(options *handlerOptions) error {
		if timeout <= 0 {
			return fmt.Errorf("providerwirev4: idle timeout must be positive")
		}
		options.idleTimeout = timeout
		return nil
	}
}

// WithMetadataExtractor configures trusted host metadata extraction.
func WithMetadataExtractor(extractor MetadataExtractor) Option {
	return func(options *handlerOptions) error {
		if extractor == nil {
			return fmt.Errorf("providerwirev4: nil metadata extractor")
		}
		options.metadataExtractor = extractor
		return nil
	}
}

// WithRequestIDGenerator configures fallback gateway request ID generation.
func WithRequestIDGenerator(generator RequestIDGenerator) Option {
	return func(options *handlerOptions) error {
		if generator == nil {
			return fmt.Errorf("providerwirev4: nil request ID generator")
		}
		options.requestIDGenerator = generator
		return nil
	}
}

// Handler serves strict LanguageModelV4 calls through a shared runtime.
type Handler struct {
	runtime               *runtime.Runtime
	maxRequestBodyBytes   int64
	maxUnaryResponseBytes int64
	maxSSEEventBytes      int64
	idleTimeout           time.Duration
	metadataExtractor     MetadataExtractor
	requestIDGenerator    RequestIDGenerator
}

// NewHandler constructs a strict runtime-backed handler.
func NewHandler(gatewayRuntime *runtime.Runtime, options ...Option) (*Handler, error) {
	if gatewayRuntime == nil {
		return nil, fmt.Errorf("providerwirev4: nil runtime")
	}
	config := handlerOptions{
		maxRequestBodyBytes:   DefaultMaxRequestBodyBytes,
		maxUnaryResponseBytes: DefaultMaxUnaryResponseBytes,
		maxSSEEventBytes:      DefaultMaxSSEEventBytes,
		idleTimeout:           DefaultIdleTimeout,
		requestIDGenerator:    defaultRequestID,
	}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("providerwirev4: nil option")
		}
		if err := option(&config); err != nil {
			return nil, err
		}
	}
	return &Handler{
		runtime:               gatewayRuntime,
		maxRequestBodyBytes:   config.maxRequestBodyBytes,
		maxUnaryResponseBytes: config.maxUnaryResponseBytes,
		maxSSEEventBytes:      config.maxSSEEventBytes,
		idleTimeout:           config.idleTimeout,
		metadataExtractor:     config.metadataExtractor,
		requestIDGenerator:    config.requestIDGenerator,
	}, nil
}

// ServeHTTP validates one request and dispatches it through the runtime.
func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	streaming, ok := handler.validateRequest(writer, request)
	if !ok {
		return
	}
	body, err := readLimited(request.Body, handler.maxRequestBodyBytes)
	if err != nil {
		if errors.Is(err, errReadLimitExceeded) {
			handler.writeProtocolError(writer, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			handler.writeProtocolError(writer, http.StatusBadRequest, "invalid request body")
		}
		return
	}
	decoded, err := DecodeCallOptions(body)
	if err != nil {
		handler.writeProtocolError(writer, http.StatusBadRequest, "invalid LanguageModelV4 request")
		return
	}
	metadata, err := handler.callMetadata(request)
	if err != nil {
		handler.writeFailure(writer, failure.Classify(failure.Wrap(failure.ErrInternal, err), failure.WithRetryable(false)))
		return
	}
	call := runtime.GatewayCall{
		Protocol:         runtime.ProtocolLanguageModelV4,
		RequestedModelID: request.Header.Get(HeaderModelID),
		CallOptions:      decoded.CallOptions,
		GatewayOptions:   decoded.GatewayOptions,
		CallMetadata:     metadata,
	}
	if streaming {
		handler.serveStream(writer, request, call)
		return
	}
	handler.serveGenerate(writer, request, call)
}

func (handler *Handler) validateRequest(writer http.ResponseWriter, request *http.Request) (bool, bool) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		handler.writeProtocolError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return false, false
	}
	if strings.TrimSpace(request.Header.Get(HeaderModelID)) == "" {
		handler.writeProtocolError(writer, http.StatusBadRequest, "model ID is required")
		return false, false
	}
	if request.Header.Get(HeaderSpecVersion) != SpecVersionV4 {
		handler.writeProtocolError(writer, http.StatusBadRequest, "unsupported specification version")
		return false, false
	}
	streamValue := request.Header.Get(HeaderStreaming)
	if streamValue != "true" && streamValue != "false" {
		handler.writeProtocolError(writer, http.StatusBadRequest, "streaming header must be true or false")
		return false, false
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != MIMEJSON {
		handler.writeProtocolError(writer, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false, false
	}
	streaming := streamValue == "true"
	responseType := MIMEJSON
	if streaming {
		responseType = MIMESSE
	}
	if !acceptsMediaType(request.Header.Values("Accept"), responseType) {
		handler.writeProtocolError(writer, http.StatusNotAcceptable, "requested response media type is not acceptable")
		return false, false
	}
	return streaming, true
}

func (handler *Handler) callMetadata(request *http.Request) (runtime.CallMetadata, error) {
	metadata := runtime.CallMetadata{}
	if handler.metadataExtractor != nil {
		var err error
		metadata, err = handler.metadataExtractor(request)
		if err != nil {
			return runtime.CallMetadata{}, fmt.Errorf("extracting trusted metadata: %w", err)
		}
	}
	if metadata.RequestID == "" {
		requestID, err := handler.requestIDGenerator()
		if err != nil {
			return runtime.CallMetadata{}, fmt.Errorf("generating request ID: %w", err)
		}
		if requestID == "" {
			return runtime.CallMetadata{}, fmt.Errorf("request ID generator returned an empty ID")
		}
		metadata.RequestID = requestID
	}
	metadata.AuthenticatedAttributes = cloneStringMap(metadata.AuthenticatedAttributes)
	return metadata, nil
}

func (handler *Handler) serveGenerate(writer http.ResponseWriter, request *http.Request, call runtime.GatewayCall) {
	outcome := handler.runtime.Generate(request.Context(), call)
	if outcome.Failure != nil {
		handler.writeFailure(writer, *outcome.Failure)
		return
	}
	result := sanitizeGenerateResult(outcome.Result)
	data, err := EncodeUnaryWithinLimit(result, handler.maxUnaryResponseBytes)
	if err != nil {
		classification := failure.Classify(failure.Wrap(failure.ErrInternal, err), failure.WithRetryable(false))
		handler.writeFailure(writer, classification)
		return
	}
	writer.Header().Set("Content-Type", MIMEJSON)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(data)
}

func (handler *Handler) serveStream(writer http.ResponseWriter, request *http.Request, call runtime.GatewayCall) {
	outcome := handler.runtime.Stream(request.Context(), call)
	if outcome.Failure != nil {
		handler.writeFailure(writer, *outcome.Failure)
		return
	}
	invocation := outcome.Invocation
	writer.Header().Set("Content-Type", MIMESSE)
	writer.Header().Set("Cache-Control", "no-cache, no-transform")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(writer)
	if err := controller.Flush(); err != nil {
		invocation.Cancel(err)
		return
	}

	for {
		part, ok, err := handler.nextPart(request.Context(), invocation)
		if err != nil {
			invocation.Cancel(err)
			if errors.Is(err, ErrIdleTimeout) {
				err = failure.Wrap(failure.ErrTimeout, err)
			}
			handler.writeLifecycleError(writer, controller, classifyLifecycleError(request.Context(), err))
			return
		}
		if !ok {
			if lifecycleErr := invocation.Wait(); lifecycleErr != nil {
				handler.writeLifecycleError(writer, controller, classifyLifecycleError(request.Context(), lifecycleErr))
			}
			return
		}
		if part.Type == provider.PartRaw && !call.CallOptions.IncludeRawChunks {
			continue
		}
		part = sanitizeStreamPart(part)
		event, err := EncodeSSEEventWithinLimit(part, handler.maxSSEEventBytes)
		if err != nil {
			invocation.Cancel(err)
			handler.writeLifecycleError(writer, controller, failure.Classify(failure.Wrap(failure.ErrInternal, err), failure.WithRetryable(false)))
			return
		}
		if _, err := writer.Write(event); err != nil {
			invocation.Cancel(err)
			return
		}
		if err := controller.Flush(); err != nil {
			invocation.Cancel(err)
			return
		}
	}
}

func (handler *Handler) nextPart(ctx context.Context, invocation *runtime.StreamInvocation) (provider.StreamPart, bool, error) {
	timer := time.NewTimer(handler.idleTimeout)
	defer timer.Stop()
	select {
	case part, ok := <-invocation.Parts():
		return part, ok, nil
	case <-timer.C:
		return provider.StreamPart{}, false, ErrIdleTimeout
	case <-ctx.Done():
		return provider.StreamPart{}, false, requestContextError(ctx)
	}
}

func requestContextError(ctx context.Context) error {
	cause := context.Cause(ctx)
	switch ctx.Err() {
	case context.DeadlineExceeded:
		return failure.Wrap(failure.ErrTimeout, cause)
	case context.Canceled:
		return failure.Wrap(failure.ErrCanceled, cause)
	default:
		return cause
	}
}

func classifyLifecycleError(ctx context.Context, err error) failure.Classification {
	var category error
	switch ctx.Err() {
	case context.DeadlineExceeded:
		category = failure.ErrTimeout
	case context.Canceled:
		category = failure.ErrCanceled
	default:
		return failure.Classify(err)
	}
	classification := failure.Classify(category)
	classification.Cause = errors.Join(requestContextError(ctx), err)
	return classification
}

func (handler *Handler) writeLifecycleError(writer http.ResponseWriter, controller *http.ResponseController, classification failure.Classification) {
	part := provider.StreamPart{Type: provider.PartError, APICallError: apiCallErrorForClassification(classification)}
	event, err := EncodeSSEEventWithinLimit(part, handler.maxSSEEventBytes)
	if err != nil {
		return
	}
	if _, err := writer.Write(event); err != nil {
		return
	}
	_ = controller.Flush()
}

func (handler *Handler) writeFailure(writer http.ResponseWriter, classification failure.Classification) {
	status, data, err := EncodeFailure(classification)
	if err != nil {
		status = http.StatusInternalServerError
		data, _ = EncodeProtocolError(status, "internal server error")
	}
	writer.Header().Set("Content-Type", MIMEJSON)
	writer.WriteHeader(status)
	_, _ = writer.Write(data)
}

func (handler *Handler) writeProtocolError(writer http.ResponseWriter, status int, message string) {
	data, err := EncodeProtocolError(status, message)
	if err != nil {
		status = http.StatusInternalServerError
		data = []byte(`{"error":{"message":"internal server error","type":"internal_server_error","statusCode":500,"isRetryable":false,"param":null}}`)
	}
	writer.Header().Set("Content-Type", MIMEJSON)
	writer.WriteHeader(status)
	_, _ = writer.Write(data)
}

func sanitizeGenerateResult(result *provider.GenerateResult) *provider.GenerateResult {
	if result == nil {
		return nil
	}
	copy := *result
	copy.Request = nil
	if result.Response != nil {
		copy.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: result.Response.ID, Timestamp: result.Response.Timestamp}}
	}
	return &copy
}

func sanitizeStreamPart(part provider.StreamPart) provider.StreamPart {
	switch part.Type {
	case provider.PartError:
		part.APICallError = sanitizePartError(part.APICallError)
	case provider.PartResponseMeta:
		part.ModelID = ""
		part.Provider = ""
		part.ResponseHeaders = nil
	}
	return part
}

func acceptsMediaType(values []string, target string) bool {
	if len(values) == 0 {
		return true
	}
	targetType, _, _ := strings.Cut(target, "/")
	bestSpecificity := -1
	bestQuality := 0.0
	for _, header := range values {
		for _, entry := range strings.Split(header, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			mediaType, parameters, err := mime.ParseMediaType(entry)
			if err != nil {
				continue
			}
			quality := 1.0
			if value, exists := parameters["q"]; exists {
				quality, err = strconv.ParseFloat(value, 64)
				if err != nil || quality < 0 || quality > 1 {
					continue
				}
				delete(parameters, "q")
			}
			if len(parameters) > 0 {
				continue
			}
			specificity := -1
			switch mediaType {
			case target:
				specificity = 2
			case targetType + "/*":
				specificity = 1
			case "*/*":
				specificity = 0
			}
			if specificity > bestSpecificity {
				bestSpecificity, bestQuality = specificity, quality
			} else if specificity == bestSpecificity && quality > bestQuality {
				bestQuality = quality
			}
		}
	}
	return bestSpecificity >= 0 && bestQuality > 0
}

var errReadLimitExceeded = errors.New("providerwirev4: read limit exceeded")

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: limit}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	var extra [1]byte
	count, extraErr := io.ReadFull(reader, extra[:])
	if count > 0 {
		return nil, errReadLimitExceeded
	}
	if extraErr != nil && !errors.Is(extraErr, io.EOF) && !errors.Is(extraErr, io.ErrUnexpectedEOF) {
		return nil, extraErr
	}
	return data, nil
}

func defaultRequestID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
