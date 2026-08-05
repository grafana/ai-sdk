package providerwire

import (
	"context"
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
	// DefaultTotalTimeout is the default maximum duration for model invocation
	// and, for streaming calls, stream consumption.
	DefaultTotalTimeout = 120 * time.Second
	// DefaultIdleTimeout is the default maximum duration between stream parts.
	DefaultIdleTimeout = 60 * time.Second
	// DefaultMaxRequestBodyBytes is the default request body limit.
	DefaultMaxRequestBodyBytes int64 = 8 << 20
)

var (
	// ErrTotalTimeout identifies expiration of the total model-call timeout.
	ErrTotalTimeout = errors.New("providerwire: total timeout")
	// ErrIdleTimeout identifies expiration of the streaming idle timeout.
	ErrIdleTimeout = errors.New("providerwire: idle timeout")
)

// ModelResolver selects a model for a validated request. The request is the
// original request and modelID is trimmed. Implementations must not retain the
// request after ResolveLanguageModel returns.
type ModelResolver interface {
	ResolveLanguageModel(r *http.Request, modelID string) (provider.LanguageModel, error)
}

// ModelResolverFunc adapts a function to [ModelResolver].
type ModelResolverFunc func(r *http.Request, modelID string) (provider.LanguageModel, error)

// ResolveLanguageModel implements [ModelResolver].
func (f ModelResolverFunc) ResolveLanguageModel(r *http.Request, modelID string) (provider.LanguageModel, error) {
	return f(r, modelID)
}

// Option configures a [Handler].
type Option func(*handlerOptions) error

type handlerOptions struct {
	totalTimeout        time.Duration
	idleTimeout         time.Duration
	maxRequestBodyBytes int64
}

// WithTotalTimeout sets the maximum duration for model invocation and stream
// consumption. The duration must be positive.
func WithTotalTimeout(timeout time.Duration) Option {
	return func(opts *handlerOptions) error {
		if timeout <= 0 {
			return fmt.Errorf("providerwire: total timeout must be positive")
		}
		opts.totalTimeout = timeout
		return nil
	}
}

// WithIdleTimeout sets the maximum duration between stream parts. The duration
// must be positive.
func WithIdleTimeout(timeout time.Duration) Option {
	return func(opts *handlerOptions) error {
		if timeout <= 0 {
			return fmt.Errorf("providerwire: idle timeout must be positive")
		}
		opts.idleTimeout = timeout
		return nil
	}
}

// WithMaxRequestBodyBytes sets the request body limit. The limit must be
// positive.
func WithMaxRequestBodyBytes(limit int64) Option {
	return func(opts *handlerOptions) error {
		if limit <= 0 {
			return fmt.Errorf("providerwire: maximum request body bytes must be positive")
		}
		opts.maxRequestBodyBytes = limit
		return nil
	}
}

// Handler serves remote [provider.LanguageModel] calls. It is path-agnostic;
// hosts mount it at [PathLanguageModel] under any host-owned prefix.
type Handler struct {
	resolver            ModelResolver
	totalTimeout        time.Duration
	idleTimeout         time.Duration
	maxRequestBodyBytes int64
}

// NewHandler constructs a provider-wire HTTP handler.
func NewHandler(resolver ModelResolver, options ...Option) (*Handler, error) {
	if isNilInterface(resolver) {
		return nil, fmt.Errorf("providerwire: nil model resolver")
	}
	opts := handlerOptions{
		totalTimeout:        DefaultTotalTimeout,
		idleTimeout:         DefaultIdleTimeout,
		maxRequestBodyBytes: DefaultMaxRequestBodyBytes,
	}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("providerwire: nil option")
		}
		if err := option(&opts); err != nil {
			return nil, err
		}
	}
	return &Handler{
		resolver:            resolver,
		totalTimeout:        opts.totalTimeout,
		idleTimeout:         opts.idleTimeout,
		maxRequestBodyBytes: opts.maxRequestBodyBytes,
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

// ServeHTTP validates and dispatches one provider-wire model call.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	modelID, streaming, apiErr := validateRequest(r)
	if apiErr != nil {
		writeAPICallErrorResponse(w, apiErr)
		return
	}

	opts, apiErr := h.decodeCallOptions(r)
	if apiErr != nil {
		writeAPICallErrorResponse(w, apiErr)
		return
	}

	model, err := h.resolver.ResolveLanguageModel(r, modelID)
	if err != nil {
		writeAPICallErrorResponse(w, normalizeAPICallError(err))
		return
	}
	if isNilInterface(model) {
		writeAPICallErrorResponse(w, internalAPICallError("model resolver returned nil model"))
		return
	}

	ctx, cancel := context.WithTimeoutCause(r.Context(), h.totalTimeout, ErrTotalTimeout)
	defer cancel()
	if streaming {
		h.serveStream(ctx, w, model, opts)
		return
	}
	h.serveUnary(ctx, w, model, opts)
}

func validateRequest(r *http.Request) (string, bool, *provider.APICallError) {
	if r.Method != http.MethodPost {
		return "", false, clientAPICallError(
			fmt.Sprintf("method %s not allowed; use POST", r.Method),
			http.StatusMethodNotAllowed,
		)
	}

	modelID := strings.TrimSpace(r.Header.Get(HeaderModelID))
	if modelID == "" {
		return "", false, clientAPICallError(fmt.Sprintf("missing required header %q", HeaderModelID), http.StatusBadRequest)
	}

	specVersion := strings.TrimSpace(r.Header.Get(HeaderSpecVersion))
	if specVersion == "" {
		return "", false, clientAPICallError(fmt.Sprintf("missing required header %q", HeaderSpecVersion), http.StatusBadRequest)
	}
	if specVersion != SpecVersionV4 {
		return "", false, clientAPICallError(
			fmt.Sprintf("unsupported %s %q; this server speaks %q", HeaderSpecVersion, specVersion, SpecVersionV4),
			http.StatusBadRequest,
		)
	}

	var streaming bool
	streamingHeader := strings.TrimSpace(r.Header.Get(HeaderStreaming))
	switch streamingHeader {
	case "true":
		streaming = true
	case "false":
		streaming = false
	case "":
		return "", false, clientAPICallError(fmt.Sprintf("missing required header %q", HeaderStreaming), http.StatusBadRequest)
	default:
		return "", false, clientAPICallError(
			fmt.Sprintf("invalid %s %q; expected \"true\" or \"false\"", HeaderStreaming, streamingHeader),
			http.StatusBadRequest,
		)
	}

	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != MIMEJSON {
			return "", false, clientAPICallError(
				fmt.Sprintf("unsupported Content-Type %q; expected %q", contentType, MIMEJSON),
				http.StatusUnsupportedMediaType,
			)
		}
	}

	if accept := r.Header.Get("Accept"); accept != "" && !acceptAllows(accept, streaming) {
		want := MIMEJSON
		if streaming {
			want = MIMESSE
		}
		return "", false, clientAPICallError(
			fmt.Sprintf("Accept %q does not allow %q", accept, want),
			http.StatusNotAcceptable,
		)
	}

	return modelID, streaming, nil
}

func acceptAllows(accept string, streaming bool) bool {
	want := MIMEJSON
	if streaming {
		want = MIMESSE
	}
	wantMain, _, _ := strings.Cut(want, "/")
	for _, item := range strings.Split(accept, ",") {
		entry, _, _ := strings.Cut(item, ";")
		entry = strings.TrimSpace(entry)
		if entry == "" || entry == "*/*" || entry == want {
			return true
		}
		mainType, subtype, ok := strings.Cut(entry, "/")
		if ok && strings.TrimSpace(mainType) == mainType && subtype == "*" && mainType == wantMain {
			return true
		}
	}
	return false
}

func (h *Handler) decodeCallOptions(r *http.Request) (provider.CallOptions, *provider.APICallError) {
	defer func() { _ = r.Body.Close() }()
	readLimit := h.maxRequestBodyBytes
	if readLimit < math.MaxInt64 {
		readLimit++
	}
	limited := io.LimitReader(r.Body, readLimit)
	body, err := io.ReadAll(limited)
	if err != nil {
		return provider.CallOptions{}, clientAPICallError(fmt.Sprintf("reading request body: %s", err), http.StatusBadRequest)
	}
	if int64(len(body)) > h.maxRequestBodyBytes {
		return provider.CallOptions{}, clientAPICallError(
			fmt.Sprintf("request body exceeds %d bytes", h.maxRequestBodyBytes),
			http.StatusRequestEntityTooLarge,
		)
	}
	opts, err := DecodeCallOptions(body)
	if err != nil {
		return provider.CallOptions{}, clientAPICallError(fmt.Sprintf("decoding %s body: %s", MIMEJSON, err), http.StatusBadRequest)
	}
	return opts, nil
}

func (h *Handler) serveUnary(ctx context.Context, w http.ResponseWriter, model provider.LanguageModel, opts provider.CallOptions) {
	result, err := model.DoGenerate(ctx, opts)
	if err != nil {
		writeAPICallErrorResponse(w, normalizeCallError(ctx, err))
		return
	}
	if result == nil {
		writeAPICallErrorResponse(w, internalAPICallError("model returned nil generate result"))
		return
	}
	body, err := EncodeGenerateResult(result)
	if err != nil {
		writeAPICallErrorResponse(w, internalAPICallError(fmt.Sprintf("encoding generate result: %s", err)))
		return
	}
	w.Header().Set("Content-Type", MIMEJSON)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *Handler) serveStream(ctx context.Context, w http.ResponseWriter, model provider.LanguageModel, opts provider.CallOptions) {
	ctx, abort := context.WithCancelCause(ctx)
	defer abort(nil)

	result, err := model.DoStream(ctx, opts)
	if err != nil {
		writeAPICallErrorResponse(w, normalizeCallError(ctx, err))
		return
	}
	if result == nil || result.Stream == nil {
		writeAPICallErrorResponse(w, internalAPICallError("model returned nil stream"))
		return
	}

	streamSetupHeaders(w)
	idle := time.NewTimer(h.idleTimeout)
	defer idle.Stop()

	for {
		select {
		case <-ctx.Done():
			h.emitFinalErrorPart(w, normalizeContextError(ctx))
			return
		case <-idle.C:
			abort(ErrIdleTimeout)
			h.emitFinalErrorPart(w, ErrIdleTimeout)
			return
		case part, open := <-result.Stream:
			if !open {
				return
			}
			if err := WriteSSEStreamPartTo(w, part); err != nil {
				abort(fmt.Errorf("providerwire: writing stream part: %w", err))
				return
			}
			resetTimer(idle, h.idleTimeout)
		}
	}
}

func streamSetupHeaders(w http.ResponseWriter) {
	header := w.Header()
	header.Set("Content-Type", MIMESSE)
	header.Set("Cache-Control", "no-cache, no-transform")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func resetTimer(timer *time.Timer, timeout time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(timeout)
}

func (h *Handler) emitFinalErrorPart(w http.ResponseWriter, err error) {
	_ = WriteSSEStreamPartTo(w, provider.StreamPart{
		Type:         provider.PartError,
		APICallError: normalizeAPICallError(err),
	})
}

func normalizeCallError(ctx context.Context, err error) *provider.APICallError {
	if apiErr, ok := preserveAPICallError(err); ok {
		return apiErr
	}
	if ctx.Err() != nil {
		return normalizeAPICallError(normalizeContextError(ctx))
	}
	return normalizeAPICallError(err)
}

func normalizeContextError(ctx context.Context) error {
	cause := context.Cause(ctx)
	if errors.Is(cause, ErrTotalTimeout) {
		return ErrTotalTimeout
	}
	if errors.Is(cause, ErrIdleTimeout) {
		return ErrIdleTimeout
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	if cause != nil {
		return cause
	}
	return ctx.Err()
}

func normalizeAPICallError(err error) *provider.APICallError {
	if apiErr, ok := preserveAPICallError(err); ok {
		return apiErr
	}
	switch {
	case errors.Is(err, ErrIdleTimeout):
		return newAPICallError("idle timeout: no stream parts produced within configured window", http.StatusGatewayTimeout, true)
	case errors.Is(err, ErrTotalTimeout), errors.Is(err, context.DeadlineExceeded):
		return newAPICallError("total timeout exceeded", http.StatusGatewayTimeout, true)
	case errors.Is(err, context.Canceled):
		return newAPICallError("consumer disconnected", 499, false)
	case err == nil:
		return internalAPICallError("unknown error")
	default:
		return newAPICallError(err.Error(), http.StatusBadGateway, true)
	}
}

func writeAPICallErrorResponse(w http.ResponseWriter, apiErr *provider.APICallError) {
	if _, err := errorResponseStatusCode(apiErr); err != nil {
		apiErr = internalAPICallError("encoding API call error response")
	} else if _, err := EncodeAPICallError(apiErr); err != nil {
		apiErr = internalAPICallError("encoding API call error response")
	}
	_ = WriteErrorResponse(w, apiErr)
}

func preserveAPICallError(err error) (*provider.APICallError, bool) {
	if err == nil {
		return nil, false
	}
	var apiErr *provider.APICallError
	if !errors.As(err, &apiErr) {
		return nil, false
	}
	if apiErr == nil {
		return internalAPICallError("nil API call error"), true
	}
	return apiErr, true
}

func clientAPICallError(message string, status int) *provider.APICallError {
	return newAPICallError(message, status, false)
}

func internalAPICallError(message string) *provider.APICallError {
	return newAPICallError(message, http.StatusInternalServerError, true)
}

func newAPICallError(message string, status int, retryable bool) *provider.APICallError {
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:     message,
		StatusCode:  status,
		IsRetryable: &retryable,
	})
}
