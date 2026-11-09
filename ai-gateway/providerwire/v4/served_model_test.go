package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

func servedMarker(to string) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartCustom, Kind: "anthropic.fallback", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"fallback","from":{"model":"claude-a"},"to":{"model":"` + to + `"}}`)}}
}

// documentOverviews returns the execution overview of every unary document or
// stream frame in body that carries one, in order.
func documentOverviews(t *testing.T, body string) []*execution.Overview {
	t.Helper()
	var documents []string
	for _, frame := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		documents = append(documents, strings.TrimPrefix(frame, "data: "))
	}
	type gateway struct {
		Gateway struct {
			Execution *execution.Overview `json:"execution"`
		} `json:"gateway"`
	}
	var overviews []*execution.Overview
	for _, document := range documents {
		var carrier struct {
			ProviderMetadata gateway `json:"providerMetadata"`
			Error            struct {
				Data struct {
					ProviderMetadata gateway `json:"providerMetadata"`
				} `json:"data"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(document), &carrier), document)
		for _, overview := range []*execution.Overview{carrier.ProviderMetadata.Gateway.Execution, carrier.Error.Data.ProviderMetadata.Gateway.Execution} {
			if overview != nil {
				overviews = append(overviews, overview)
			}
		}
	}
	return overviews
}

func servedModels(overviews []*execution.Overview) []string {
	served := make([]string, len(overviews))
	for i, overview := range overviews {
		served[i] = overview.Attempts[len(overview.Attempts)-1].ServedModelID
	}
	return served
}

// blockingStream sends parts, calls after, then holds the stream open until
// ctx ends.
func blockingStream(ctx context.Context, after func(), parts ...provider.StreamPart) <-chan provider.StreamPart {
	stream := make(chan provider.StreamPart)
	go func() {
		defer close(stream)
		for _, part := range parts {
			select {
			case stream <- part:
			case <-ctx.Done():
				return
			}
		}
		if after != nil {
			after()
		}
		<-ctx.Done()
	}()
	return stream
}

func TestServedModel_StreamCarriers(t *testing.T) {
	apiError := provider.NewAPICallError(provider.APICallErrorOptions{Message: "busy", StatusCode: http.StatusServiceUnavailable})
	invalidFinish := finishPart()
	invalidFinish.FinishReason = &provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "\xff"}
	hop := []provider.StreamPart{segmentMeta("msg"), servedMarker("claude-b")}
	for _, tc := range []struct {
		name     string
		parts    []provider.StreamPart
		block    bool
		idle     time.Duration
		deadline time.Duration
		cancel   bool
	}{
		{name: "committed error part", parts: append(slices.Clone(hop), provider.StreamPart{Type: provider.PartError, APICallError: apiError}, finishPart())},
		{name: "closed", parts: hop},
		{name: "invalid output", parts: append(slices.Clone(hop), provider.StreamPart{Type: provider.PartTextDelta, ID: "missing", Delta: "x"})},
		{name: "adapter failure", parts: append(slices.Clone(hop), invalidFinish)},
		{name: "idle timeout", parts: hop, block: true, idle: 50 * time.Millisecond},
		{name: "deadline", parts: hop, block: true, deadline: 100 * time.Millisecond},
		{name: "canceled", parts: hop, block: true, cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			if tc.idle > 0 {
				limits.StreamIdleDuration = tc.idle
			}
			request := streamRequest(`{"prompt":[]}`)
			if tc.deadline > 0 {
				request = withRequestDeadline(t, request, tc.deadline)
			}
			ctx, cancel := context.WithCancel(request.Context())
			defer cancel()
			request = request.WithContext(ctx)
			var after func()
			if tc.cancel {
				after = cancel
			}
			model := &recordingModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				if tc.block {
					return &provider.StreamResult{Stream: blockingStream(ctx, after, tc.parts...)}, nil
				}
				return &provider.StreamResult{Stream: makeStream(tc.parts...)}, nil
			}}
			response := invocationHarness(t, limits, model).serve(request)
			require.Equal(t, http.StatusOK, response.Code)
			body := response.Body.String()
			overviews := documentOverviews(t, body)
			require.NotEmpty(t, overviews, body)
			assert.Equal(t, slices.Repeat([]string{"claude-b"}, len(overviews)), servedModels(overviews), body)
			assert.Contains(t, body, `"type":"error"`)
		})
	}
}

func TestServedModel_UnaryCarriers(t *testing.T) {
	marker := provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "anthropic.fallback", ProviderMetadata: servedMarker("claude-b").ProviderMetadata}
	for _, tc := range []struct {
		name   string
		result func() *provider.GenerateResult
		status int
	}{
		{"result", func() *provider.GenerateResult {
			result := validGenerateResult()
			result.Content = append([]provider.GenerateContentPart{marker}, result.Content...)
			return result
		}, http.StatusOK},
		{"invalid output", func() *provider.GenerateResult {
			result := validGenerateResult()
			result.Content = append([]provider.GenerateContentPart{marker}, provider.GenerateContentPart{Type: provider.ContentText, Text: "x", ProviderExecuted: true})
			return result
		}, http.StatusInternalServerError},
		{"mapping error", func() *provider.GenerateResult {
			result := validGenerateResult()
			result.Content = append([]provider.GenerateContentPart{marker}, result.Content...)
			result.FinishReason.Raw = "\xff"
			return result
		}, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return tc.result(), nil }}
			response := invocationHarness(t, testLimits(), model).serve(validRequest(`{"prompt":[]}`))
			require.Equal(t, tc.status, response.Code, response.Body.String())
			overviews := documentOverviews(t, response.Body.String())
			require.Len(t, overviews, 1, response.Body.String())
			assert.Equal(t, []string{"claude-b"}, servedModels(overviews))
		})
	}
}

func TestStreamingSegments_ExecutionOverviewServedModel(t *testing.T) {
	model := &recordingModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(joinSegmentParts(
			[]provider.StreamPart{segmentMeta("msg_1"), servedMarker("claude-b")},
			segmentText("0", "a"),
			[]provider.StreamPart{finishPart(), segmentMeta("msg_2"), servedMarker("claude-c")},
			segmentText("0", "b"),
			[]provider.StreamPart{finishPart()},
		)...)}, nil
	}}
	response := invocationHarness(t, testLimits(), model).serve(streamRequest(`{"prompt":[]}`))
	require.Equal(t, http.StatusOK, response.Code)
	requireStreamBodyMatchesSchema(t, response.Body.String())
	assert.Equal(t, []string{"claude-b", "claude-c"}, servedModels(documentOverviews(t, response.Body.String())))
}
