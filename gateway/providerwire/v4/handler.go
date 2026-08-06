package providerwirev4

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

const (
	// DefaultTotalTimeout is the default maximum model-resolution and invocation duration.
	DefaultTotalTimeout = 120 * time.Second
	// DefaultIdleTimeout is the default maximum wait between provider stream parts.
	DefaultIdleTimeout = 60 * time.Second
	// DefaultMaxRequestBodyBytes is the default bounded request read size.
	DefaultMaxRequestBodyBytes int64 = 8 << 20
	// DefaultMaxUnaryResponseBytes is the default encoded unary commitment limit.
	DefaultMaxUnaryResponseBytes int64 = 16 << 20
	// DefaultMaxSSEEventBytes is the default complete framed event limit.
	DefaultMaxSSEEventBytes int64 = 8 << 20
)

var (
	// ErrTotalTimeout identifies expiration of the handler's total lifecycle timeout.
	ErrTotalTimeout = errors.New("providerwirev4: total timeout")
	// ErrIdleTimeout identifies provider inactivity while waiting for a part.
	ErrIdleTimeout = errors.New("providerwirev4: stream idle timeout")
)

// Option configures a Handler.
type Option func(*handlerOptions) error

type handlerOptions struct {
	totalTimeout          time.Duration
	idleTimeout           time.Duration
	maxRequestBodyBytes   int64
	maxUnaryResponseBytes int64
	maxSSEEventBytes      int64
}

// WithTotalTimeout sets the maximum model-resolution and invocation duration.
func WithTotalTimeout(timeout time.Duration) Option {
	return func(options *handlerOptions) error {
		if timeout <= 0 {
			return fmt.Errorf("providerwirev4: total timeout must be positive")
		}
		options.totalTimeout = timeout
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

// Handler serves strict LanguageModelV4 calls through a gateway model catalog.
type Handler struct {
	resolver              catalog.ModelResolver
	totalTimeout          time.Duration
	idleTimeout           time.Duration
	maxRequestBodyBytes   int64
	maxUnaryResponseBytes int64
	maxSSEEventBytes      int64
}

// NewHandler constructs a strict catalog-backed handler.
func NewHandler(resolver catalog.ModelResolver, options ...Option) (*Handler, error) {
	if isNilInterface(resolver) {
		return nil, fmt.Errorf("providerwirev4: nil model resolver")
	}
	config := handlerOptions{
		totalTimeout:          DefaultTotalTimeout,
		idleTimeout:           DefaultIdleTimeout,
		maxRequestBodyBytes:   DefaultMaxRequestBodyBytes,
		maxUnaryResponseBytes: DefaultMaxUnaryResponseBytes,
		maxSSEEventBytes:      DefaultMaxSSEEventBytes,
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
		resolver: resolver, totalTimeout: config.totalTimeout, idleTimeout: config.idleTimeout,
		maxRequestBodyBytes: config.maxRequestBodyBytes, maxUnaryResponseBytes: config.maxUnaryResponseBytes,
		maxSSEEventBytes: config.maxSSEEventBytes,
	}, nil
}

// ServeHTTP validates one request, resolves its exact model ID, and invokes the model.
func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	modelID, streaming, ok := handler.validateRequest(writer, request)
	if !ok {
		return
	}
	defer func() { _ = request.Body.Close() }()
	body, err := readLimited(request.Body, handler.maxRequestBodyBytes)
	if err != nil {
		if errors.Is(err, errReadLimitExceeded) {
			handler.writeProtocolError(writer, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			handler.writeProtocolError(writer, http.StatusBadRequest, "invalid request body")
		}
		return
	}
	options, err := decodeCallOptionsJSON(body)
	if err != nil {
		handler.writeProtocolError(writer, http.StatusBadRequest, "invalid LanguageModelV4 request")
		return
	}
	if options.IncludeRawChunks {
		handler.writeProtocolError(writer, http.StatusBadRequest, "raw chunks are not supported")
		return
	}

	totalCtx, totalCancel := context.WithTimeoutCause(request.Context(), handler.totalTimeout, ErrTotalTimeout)
	defer totalCancel()
	resolved, err := handler.resolver.ResolveModel(totalCtx, modelID)
	if err != nil {
		handler.writeFailure(writer, classifyResolverError(totalCtx, err, modelID))
		return
	}
	if cause := context.Cause(totalCtx); cause != nil {
		handler.writeFailure(writer, classifyResolverError(totalCtx, cause, modelID))
		return
	}
	if isNilInterface(resolved.Model) {
		handler.writeFailure(writer, internalFailure(errors.New("providerwirev4: model resolver returned a nil model")))
		return
	}
	if streaming {
		handler.serveStream(totalCtx, writer, resolved.Model, options)
		return
	}
	handler.serveGenerate(totalCtx, writer, resolved.Model, options)
}

func (handler *Handler) validateRequest(writer http.ResponseWriter, request *http.Request) (string, bool, bool) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		handler.writeProtocolError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return "", false, false
	}
	modelID := request.Header.Get(HeaderModelID)
	if strings.TrimSpace(modelID) == "" {
		handler.writeProtocolError(writer, http.StatusBadRequest, "model ID is required")
		return "", false, false
	}
	if request.Header.Get(HeaderSpecVersion) != SpecVersionV4 {
		handler.writeProtocolError(writer, http.StatusBadRequest, "unsupported specification version")
		return "", false, false
	}
	streamValue := request.Header.Get(HeaderStreaming)
	if streamValue != "true" && streamValue != "false" {
		handler.writeProtocolError(writer, http.StatusBadRequest, "streaming header must be true or false")
		return "", false, false
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != MIMEJSON {
		handler.writeProtocolError(writer, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return "", false, false
	}
	streaming := streamValue == "true"
	responseType := MIMEJSON
	if streaming {
		responseType = MIMESSE
	}
	if !acceptsMediaType(request.Header.Values("Accept"), responseType) {
		handler.writeProtocolError(writer, http.StatusNotAcceptable, "requested response media type is not acceptable")
		return "", false, false
	}
	return modelID, streaming, true
}

func (handler *Handler) serveGenerate(ctx context.Context, writer http.ResponseWriter, model provider.LanguageModel, options provider.CallOptions) {
	result, err := model.DoGenerate(ctx, options)
	if err != nil {
		handler.writeFailure(writer, classifyInvocationError(ctx, err))
		return
	}
	if context.Cause(ctx) != nil {
		handler.writeFailure(writer, classifyInvocationError(ctx, context.Cause(ctx)))
		return
	}
	if result == nil {
		handler.writeFailure(writer, internalFailure(errors.New("providerwirev4: model returned a nil generate result")))
		return
	}
	data, err := encodeUnaryWithinLimit(sanitizeGenerateResult(result), handler.maxUnaryResponseBytes)
	if err != nil {
		handler.writeFailure(writer, internalFailure(err))
		return
	}
	writer.Header().Set("Content-Type", MIMEJSON)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(data)
}

func (handler *Handler) serveStream(totalCtx context.Context, writer http.ResponseWriter, model provider.LanguageModel, options provider.CallOptions) {
	streamCtx, cancel := context.WithCancelCause(totalCtx)
	defer cancel(nil)
	result, err := model.DoStream(streamCtx, options)
	if err != nil {
		handler.writeFailure(writer, classifyInvocationError(streamCtx, err))
		return
	}
	if context.Cause(streamCtx) != nil {
		handler.writeFailure(writer, classifyInvocationError(streamCtx, context.Cause(streamCtx)))
		return
	}
	if result == nil || result.Stream == nil {
		handler.writeFailure(writer, internalFailure(errors.New("providerwirev4: model returned a nil stream")))
		return
	}

	writer.Header().Set("Content-Type", MIMESSE)
	writer.Header().Set("Cache-Control", "no-cache, no-transform")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(writer)
	if err := controller.Flush(); err != nil {
		cancel(err)
		return
	}

	for {
		part, open, waitErr := handler.nextPart(streamCtx, result.Stream)
		if waitErr != nil {
			cancel(waitErr)
			failure := classifyInvocationError(streamCtx, waitErr)
			if errors.Is(waitErr, ErrIdleTimeout) {
				failure = timeoutFailure(waitErr)
			}
			handler.writeLifecycleError(writer, controller, failure)
			return
		}
		if !open {
			return
		}
		if part.Type == provider.PartRaw {
			continue
		}
		part = sanitizeStreamPart(part)
		event, err := encodeSSEEventWithinLimit(part, handler.maxSSEEventBytes)
		if err != nil {
			cancel(err)
			handler.writeLifecycleError(writer, controller, internalFailure(err))
			return
		}
		if _, err := writer.Write(event); err != nil {
			cancel(err)
			return
		}
		if err := controller.Flush(); err != nil {
			cancel(err)
			return
		}
	}
}

func (handler *Handler) nextPart(ctx context.Context, parts <-chan provider.StreamPart) (provider.StreamPart, bool, error) {
	if cause := context.Cause(ctx); cause != nil {
		return provider.StreamPart{}, false, cause
	}
	timer := time.NewTimer(handler.idleTimeout)
	defer timer.Stop()
	select {
	case part, open := <-parts:
		if cause := context.Cause(ctx); cause != nil {
			return provider.StreamPart{}, false, cause
		}
		return part, open, nil
	case <-timer.C:
		if cause := context.Cause(ctx); cause != nil {
			return provider.StreamPart{}, false, cause
		}
		return provider.StreamPart{}, false, ErrIdleTimeout
	case <-ctx.Done():
		return provider.StreamPart{}, false, context.Cause(ctx)
	}
}

func (handler *Handler) writeLifecycleError(writer http.ResponseWriter, controller *http.ResponseController, failure safeFailure) {
	part := provider.StreamPart{Type: provider.PartError, APICallError: apiCallErrorForFailure(failure)}
	event, err := encodeSSEEventWithinLimit(part, handler.maxSSEEventBytes)
	if err != nil {
		return
	}
	if _, err := writer.Write(event); err != nil {
		return
	}
	_ = controller.Flush()
}

func (handler *Handler) writeFailure(writer http.ResponseWriter, failure safeFailure) {
	status, data, err := encodeFailure(failure)
	if err != nil {
		status = http.StatusInternalServerError
		data, _ = encodeProtocolError(status, "internal server error")
	}
	writer.Header().Set("Content-Type", MIMEJSON)
	writer.WriteHeader(status)
	_, _ = writer.Write(data)
}

func (handler *Handler) writeProtocolError(writer http.ResponseWriter, status int, message string) {
	data, err := encodeProtocolError(status, message)
	if err != nil {
		status = http.StatusInternalServerError
		data = []byte(`{"error":{"message":"internal server error","type":"internal_server_error","statusCode":500,"isRetryable":false,"param":null}}`)
	}
	writer.Header().Set("Content-Type", MIMEJSON)
	writer.WriteHeader(status)
	_, _ = writer.Write(data)
}

func sanitizeGenerateResult(result *provider.GenerateResult) *provider.GenerateResult {
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
