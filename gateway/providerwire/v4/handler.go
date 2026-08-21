package v4

import (
	"context"
	"net/http"
	"unicode/utf8"
)

type modelCallResult[T any] struct {
	result T
	err    error
}

func awaitModelCall[T any](ctx context.Context, call func() (T, error)) (modelCallResult[T], bool) {
	resultChannel := make(chan modelCallResult[T], 1)
	go func() {
		result, err := call()
		resultChannel <- modelCallResult[T]{result: result, err: err}
	}()
	select {
	case <-ctx.Done():
		return modelCallResult[T]{}, false
	case result := <-resultChannel:
		if _, canceled := contextFailure(ctx); canceled {
			return modelCallResult[T]{}, false
		}
		return result, true
	}
}

// ServeHTTP validates and serves one strict ProviderWire V4 model call.
func (h *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	envelope, value, ok := validateEnvelope(request)
	if !ok {
		h.writeError(w, value)
		return
	}
	options, value, ok := h.readAndMapRequest(request.Context(), request.Body)
	if !ok {
		h.writeError(w, value)
		return
	}
	mode := CallModeUnary
	if envelope.streaming {
		mode = CallModeStream
	}
	if h.policy != nil {
		policyFailure := h.policy.Check(request.Context(), PolicyRequest{
			ModelID: envelope.modelID,
			Mode:    mode,
			Options: options,
		})
		if value, canceled := contextFailure(request.Context()); canceled {
			h.writeError(w, value)
			return
		}
		if policyFailure != nil {
			h.writeError(w, *policyFailure)
			return
		}
	}
	resolved, err := h.resolver.ResolveModel(request.Context(), envelope.modelID)
	if value, canceled := contextFailure(request.Context()); canceled {
		h.writeError(w, value)
		return
	}
	if err != nil {
		h.writeError(w, reduceResolverError(request.Context(), err))
		return
	}
	if resolved.ID == "" || !utf8.ValidString(resolved.ID) || isNilInterface(resolved.Model) {
		h.writeError(w, canonicalInternal)
		return
	}
	callContext, cancel := context.WithTimeout(request.Context(), h.totalTimeout)
	defer cancel()
	if envelope.streaming {
		h.serveStream(w, resolved.Model, options, resolved.ID, callContext, cancel)
		return
	}
	h.serveUnary(w, resolved.Model, options, resolved.ID, callContext)
}
