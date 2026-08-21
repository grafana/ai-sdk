// Package v4 provides the strict ProviderWire V4 HTTP adapter.
package v4

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

const (
	// PathLanguageModel is the protocol-relative language-model route.
	PathLanguageModel = "/language-model"
	// HeaderModelID selects the public model ID.
	HeaderModelID = "ai-language-model-id"
	// HeaderSpecVersion selects the ProviderWire specification version.
	HeaderSpecVersion = "ai-language-model-specification-version"
	// HeaderStreaming selects streaming or unary mode.
	HeaderStreaming = "ai-language-model-streaming"
	// SpecVersionV4 is the accepted ProviderWire specification version.
	SpecVersionV4 = "4"
	// MIMEJSON is the exact JSON media type accepted and emitted by the adapter.
	MIMEJSON = "application/json"
	// MIMESSE is the Server-Sent Events media type emitted for streams.
	MIMESSE = "text/event-stream"

	// DefaultMaxRequestBodyBytes is the default maximum request body size.
	DefaultMaxRequestBodyBytes int64 = 8 << 20
	// DefaultMaxUnaryResponseBytes is the default maximum unary response size.
	DefaultMaxUnaryResponseBytes int64 = 8 << 20
	// DefaultMaxErrorResponseBytes is the default maximum JSON error response size.
	DefaultMaxErrorResponseBytes int64 = 64 << 10
	// DefaultMaxEventBytes is the default maximum complete SSE event size.
	DefaultMaxEventBytes int64 = 1 << 20
	// DefaultTotalTimeout is the default total model call timeout.
	DefaultTotalTimeout = 120 * time.Second
	// DefaultIdleTimeout is the default interval allowed between stream parts.
	DefaultIdleTimeout = 60 * time.Second
)

//go:embed schema/*.json
var schemaFiles embed.FS

// CallMode identifies the requested model-call mode.
type CallMode string

const (
	// CallModeUnary identifies a unary model call.
	CallModeUnary CallMode = "unary"
	// CallModeStream identifies a streaming model call.
	CallModeStream CallMode = "stream"
)

// PolicyRequest is the protocol-neutral input to a host policy.
type PolicyRequest struct {
	ModelID string
	Mode    CallMode
	Options provider.CallOptions
}

// Policy can reject a validated and mapped call before catalog resolution.
// Implementations are trusted not to mutate or retain aliased data in request.
type Policy interface {
	Check(ctx context.Context, request PolicyRequest) *failure.Failure
}

// PolicyFunc adapts a function to Policy.
type PolicyFunc func(ctx context.Context, request PolicyRequest) *failure.Failure

// Check implements Policy.
func (f PolicyFunc) Check(ctx context.Context, request PolicyRequest) *failure.Failure {
	return f(ctx, request)
}

// Option configures a Handler.
type Option func(*handlerOptions) error

type handlerOptions struct {
	policy                Policy
	maxRequestBodyBytes   int64
	maxUnaryResponseBytes int64
	maxErrorResponseBytes int64
	maxEventBytes         int64
	totalTimeout          time.Duration
	idleTimeout           time.Duration
}

// WithPolicy configures the optional host policy.
func WithPolicy(policy Policy) Option {
	return func(options *handlerOptions) error {
		if isNilInterface(policy) {
			return fmt.Errorf("providerwire v4: nil policy")
		}
		options.policy = policy
		return nil
	}
}

// WithMaxRequestBodyBytes sets the request body limit.
func WithMaxRequestBodyBytes(limit int64) Option {
	return positiveLimitOption("maximum request body bytes", limit, func(options *handlerOptions) { options.maxRequestBodyBytes = limit })
}

// WithMaxUnaryResponseBytes sets the unary response limit.
func WithMaxUnaryResponseBytes(limit int64) Option {
	return positiveLimitOption("maximum unary response bytes", limit, func(options *handlerOptions) { options.maxUnaryResponseBytes = limit })
}

// WithMaxErrorResponseBytes sets the complete JSON error response limit.
func WithMaxErrorResponseBytes(limit int64) Option {
	return positiveLimitOption("maximum error response bytes", limit, func(options *handlerOptions) { options.maxErrorResponseBytes = limit })
}

// WithMaxEventBytes sets the complete SSE event limit.
func WithMaxEventBytes(limit int64) Option {
	return positiveLimitOption("maximum event bytes", limit, func(options *handlerOptions) { options.maxEventBytes = limit })
}

func positiveLimitOption(name string, limit int64, set func(*handlerOptions)) Option {
	return func(options *handlerOptions) error {
		if limit <= 0 {
			return fmt.Errorf("providerwire v4: %s must be positive", name)
		}
		set(options)
		return nil
	}
}

// WithTotalTimeout sets the total model call timeout.
func WithTotalTimeout(timeout time.Duration) Option {
	return durationOption("total timeout", timeout, func(options *handlerOptions) { options.totalTimeout = timeout })
}

// WithIdleTimeout sets the stream idle timeout.
func WithIdleTimeout(timeout time.Duration) Option {
	return durationOption("idle timeout", timeout, func(options *handlerOptions) { options.idleTimeout = timeout })
}

func durationOption(name string, timeout time.Duration, set func(*handlerOptions)) Option {
	return func(options *handlerOptions) error {
		if timeout <= 0 {
			return fmt.Errorf("providerwire v4: %s must be positive", name)
		}
		set(options)
		return nil
	}
}

type compiledSchemas struct {
	request *schema.CompiledSchema
	error   *schema.CompiledSchema
	unary   *schema.CompiledSchema
	stream  *schema.CompiledSchema
}

func loadSchemas() (compiledSchemas, error) {
	load := func(name string) (*schema.CompiledSchema, error) {
		data, err := schemaFiles.ReadFile("schema/" + name)
		if err != nil {
			return nil, fmt.Errorf("providerwire v4: reading embedded schema %s: %w", name, err)
		}
		compiled, err := schema.CompileSchema(json.RawMessage(data))
		if err != nil {
			return nil, fmt.Errorf("providerwire v4: compiling embedded schema %s: %w", name, err)
		}
		return compiled, nil
	}
	request, err := load("providerwire-v4-request.schema.json")
	if err != nil {
		return compiledSchemas{}, err
	}
	errorSchema, err := load("error-response.schema.json")
	if err != nil {
		return compiledSchemas{}, err
	}
	unary, err := load("unary-response.schema.json")
	if err != nil {
		return compiledSchemas{}, err
	}
	stream, err := load("stream-part.schema.json")
	if err != nil {
		return compiledSchemas{}, err
	}
	return compiledSchemas{request: request, error: errorSchema, unary: unary, stream: stream}, nil
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

// Handler serves strict ProviderWire V4 language-model requests.
type Handler struct {
	resolver              catalog.ModelResolver
	policy                Policy
	maxRequestBodyBytes   int64
	maxUnaryResponseBytes int64
	maxErrorResponseBytes int64
	maxEventBytes         int64
	totalTimeout          time.Duration
	idleTimeout           time.Duration
	schemas               compiledSchemas
}

// NewHandler constructs a strict ProviderWire V4 handler.
func NewHandler(resolver catalog.ModelResolver, options ...Option) (*Handler, error) {
	if isNilInterface(resolver) {
		return nil, fmt.Errorf("providerwire v4: nil model resolver")
	}
	opts := handlerOptions{
		maxRequestBodyBytes:   DefaultMaxRequestBodyBytes,
		maxUnaryResponseBytes: DefaultMaxUnaryResponseBytes,
		maxErrorResponseBytes: DefaultMaxErrorResponseBytes,
		maxEventBytes:         DefaultMaxEventBytes,
		totalTimeout:          DefaultTotalTimeout,
		idleTimeout:           DefaultIdleTimeout,
	}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("providerwire v4: nil option")
		}
		if err := option(&opts); err != nil {
			return nil, err
		}
	}
	schemas, err := loadSchemas()
	if err != nil {
		return nil, err
	}
	if opts.maxErrorResponseBytes < int64(len(canonicalErrorBytes)) {
		return nil, fmt.Errorf("providerwire v4: maximum error response bytes cannot fit canonical fallback")
	}
	if opts.maxEventBytes < int64(len(canonicalErrorFrame)) {
		return nil, fmt.Errorf("providerwire v4: maximum event bytes cannot fit canonical fallback")
	}
	return &Handler{
		resolver:              resolver,
		policy:                opts.policy,
		maxRequestBodyBytes:   opts.maxRequestBodyBytes,
		maxUnaryResponseBytes: opts.maxUnaryResponseBytes,
		maxErrorResponseBytes: opts.maxErrorResponseBytes,
		maxEventBytes:         opts.maxEventBytes,
		totalTimeout:          opts.totalTimeout,
		idleTimeout:           opts.idleTimeout,
		schemas:               schemas,
	}, nil
}
