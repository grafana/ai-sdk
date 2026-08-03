package openai

import (
	"context"
	"net/http"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

func consumeStream(ctx context.Context, items <-chan responseStreamItem, buffered []responses.ResponseStreamEventUnion, ch chan<- provider.StreamPart, warnings []provider.Warning, br buildResult, requestBody responses.ResponseNewParams, response *http.Response, generateID func() string, providerName string) {
	parts := make(chan provider.StreamPart, 64)
	go func() {
		defer close(parts)
		consumeStreamParts(items, buffered, parts, warnings, br, requestBody, response, generateID, providerName)
	}()

	for {
		select {
		case part, ok := <-parts:
			if !ok {
				return
			}
			select {
			case ch <- part:
			case <-ctx.Done():
				drainProviderStreamParts(parts)
				return
			}
		case <-ctx.Done():
			drainProviderStreamParts(parts)
			return
		}
	}
}

func consumeStreamParts(items <-chan responseStreamItem, buffered []responses.ResponseStreamEventUnion, ch chan<- provider.StreamPart, warnings []provider.Warning, br buildResult, requestBody responses.ResponseNewParams, response *http.Response, generateID func() string, providerName string) {
	adapter := newStreamAdapter(warnings, br, requestBody, response, generateID, providerName)

	adapter.startEmitted = true
	ch <- provider.StreamPart{Type: provider.PartStreamStart, Warnings: warnings}

	for _, event := range buffered {
		adapter.handleEvent(event, ch)
	}
	for item := range items {
		if item.err != nil {
			wrapped := wrapStreamTransportError(item.err, requestBody, response)
			apiErr, ok := wrapped.(*provider.APICallError)
			if !ok {
				apiErr = provider.NewAPICallError(provider.APICallErrorOptions{Message: wrapped.Error(), Cause: wrapped})
			}
			adapter.encounteredStreamError = true
			ch <- provider.StreamPart{Type: provider.PartError, APICallError: apiErr}
			continue
		}
		if item.event != nil {
			adapter.handleEvent(*item.event, ch)
		}
	}
	adapter.emitPendingErrorFinish(ch)
}

func drainProviderStreamParts(parts <-chan provider.StreamPart) {
	go func() {
		for range parts {
		}
	}()
}
