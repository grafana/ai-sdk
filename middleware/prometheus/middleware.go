package prometheus

import (
	"context"
	"time"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

const (
	operationGenerate = "generate"
	operationStream   = "stream"
)

type instrumentation struct {
	config     config
	collectors *collectors
}

// Middleware constructs Prometheus collectors and returns reusable language
// model middleware. The returned middleware is safe for concurrent use.
func Middleware(opts Options) (middleware.Middleware, error) {
	inst, err := newInstrumentation(opts)
	if err != nil {
		return middleware.Middleware{}, err
	}
	return middleware.Middleware{
		WrapGenerate: inst.wrapGenerate,
		WrapStream:   inst.wrapStream,
	}, nil
}

// MustMiddleware is like Middleware but panics when collector registration fails.
func MustMiddleware(opts Options) middleware.Middleware {
	mw, err := Middleware(opts)
	if err != nil {
		panic(err)
	}
	return mw
}

// Wrap constructs Prometheus middleware and applies it to base.
func Wrap(base provider.LanguageModel, opts Options) (provider.LanguageModel, error) {
	mw, err := Middleware(opts)
	if err != nil {
		return nil, err
	}
	return middleware.Wrap(middleware.WrapOptions{Model: base, Middleware: []middleware.Middleware{mw}}), nil
}

// MustWrap is like Wrap but panics when collector registration fails.
func MustWrap(base provider.LanguageModel, opts Options) provider.LanguageModel {
	model, err := Wrap(base, opts)
	if err != nil {
		panic(err)
	}
	return model
}

func newInstrumentation(opts Options) (*instrumentation, error) {
	config := normalizeOptions(opts)
	collectors := newCollectors(config)
	if err := collectors.register(config.registerer); err != nil {
		return nil, err
	}
	return &instrumentation{config: config, collectors: collectors}, nil
}

func (i *instrumentation) wrapGenerate(ctx context.Context, p middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
	requested := i.requestedIdentity(p.Model)
	start := time.Now()
	i.collectors.inflight.WithLabelValues(operationGenerate, requested.provider, requested.model).Inc()
	defer i.collectors.inflight.WithLabelValues(operationGenerate, requested.provider, requested.model).Dec()

	result, err := p.DoGenerate(ctx)
	duration := time.Since(start).Seconds()
	if err != nil {
		out := classifyError(err)
		i.observeRequest(operationGenerate, requested, out, duration)
		return result, err
	}

	finalID := i.generateFinalIdentity(requested, result)
	out := outcome{status: statusSuccess, errorType: errorTypeNone, statusCode: statusCodeNone, finishReason: finishReasonNone}
	if result != nil {
		out = successOutcome(result.FinishReason)
	}
	i.observeRequest(operationGenerate, finalID, out, duration)
	if result != nil {
		i.observeUsage(operationGenerate, finalID, result.Usage)
	}
	return result, nil
}

func (i *instrumentation) wrapStream(ctx context.Context, p middleware.WrapStreamParams) (*provider.StreamResult, error) {
	requested := i.requestedIdentity(p.Model)
	start := time.Now()
	i.collectors.inflight.WithLabelValues(operationStream, requested.provider, requested.model).Inc()

	result, err := p.DoStream(ctx)
	if err != nil {
		duration := time.Since(start).Seconds()
		out := classifyError(err)
		i.observeRequest(operationStream, requested, out, duration)
		i.collectors.inflight.WithLabelValues(operationStream, requested.provider, requested.model).Dec()
		return result, err
	}
	if result == nil || result.Stream == nil {
		i.observeRequest(operationStream, requested, outcome{status: statusSuccess, errorType: errorTypeNone, statusCode: statusCodeNone, finishReason: finishReasonNone}, time.Since(start).Seconds())
		i.collectors.inflight.WithLabelValues(operationStream, requested.provider, requested.model).Dec()
		return result, nil
	}

	teeCh := make(chan provider.StreamPart, streamBufferSize)
	teeResult := &provider.StreamResult{
		Stream:   teeCh,
		Request:  result.Request,
		Response: result.Response,
	}
	go i.runStreamTee(ctx, result.Stream, teeCh, requested, start)
	return teeResult, nil
}

func (i *instrumentation) observeRequest(operation string, id identity, out outcome, duration float64) {
	i.collectors.requests.WithLabelValues(operation, id.provider, id.model, out.status, out.errorType, out.statusCode, out.finishReason).Inc()
	i.collectors.duration.WithLabelValues(operation, id.provider, id.model, out.status).Observe(duration)
}
