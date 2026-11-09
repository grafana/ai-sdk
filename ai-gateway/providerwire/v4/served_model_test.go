package v4

import (
	"context"
	"encoding/json"
	"net/http"
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
func documentOverviews(t *testing.T, body string, streaming bool) []*execution.Overview {
	t.Helper()
	documents := []string{body}
	if streaming {
		documents = nil
		for _, frame := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
			documents = append(documents, strings.TrimPrefix(frame, "data: "))
		}
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
	for _, tc := range []struct {
		name    string
		limits  func(*execution.Limits)
		request func(*testing.T, *http.Request) (*http.Request, func())
		parts   func(ctx context.Context, cancel func()) <-chan provider.StreamPart
		code    string
	}{
		{name: "committed error part", parts: func(context.Context, func()) <-chan provider.StreamPart {
			return makeStream(segmentMeta("msg"), servedMarker("claude-b"), provider.StreamPart{Type: provider.PartError, APICallError: apiError}, finishPart())
		}},
		{name: "closed", parts: func(context.Context, func()) <-chan provider.StreamPart {
			return makeStream(segmentMeta("msg"), servedMarker("claude-b"))
		}},
		{name: "invalid output", parts: func(context.Context, func()) <-chan provider.StreamPart {
			return makeStream(segmentMeta("msg"), servedMarker("claude-b"), provider.StreamPart{Type: provider.PartTextDelta, ID: "missing", Delta: "x"})
		}},
		{name: "adapter failure", parts: func(context.Context, func()) <-chan provider.StreamPart {
			return makeStream(segmentMeta("msg"), servedMarker("claude-b"), invalidFinish)
		}},
		{name: "idle timeout", limits: func(limits *execution.Limits) { limits.StreamIdleDuration = 50 * time.Millisecond }, parts: func(ctx context.Context, _ func()) <-chan provider.StreamPart {
			return blockingStream(ctx, nil, segmentMeta("msg"), servedMarker("claude-b"))
		}},
		{name: "deadline", request: func(t *testing.T, r *http.Request) (*http.Request, func()) {
			return withRequestDeadline(t, r, 100*time.Millisecond), func() {}
		}, parts: func(ctx context.Context, _ func()) <-chan provider.StreamPart {
			return blockingStream(ctx, nil, segmentMeta("msg"), servedMarker("claude-b"))
		}},
		{name: "canceled", request: func(_ *testing.T, r *http.Request) (*http.Request, func()) {
			ctx, cancel := context.WithCancel(r.Context())
			return r.WithContext(ctx), cancel
		}, parts: func(ctx context.Context, cancel func()) <-chan provider.StreamPart {
			return blockingStream(ctx, cancel, segmentMeta("msg"), servedMarker("claude-b"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			if tc.limits != nil {
				tc.limits(&limits)
			}
			request, cancel := streamRequest(`{"prompt":[]}`), func() {}
			if tc.request != nil {
				request, cancel = tc.request(t, request)
			}
			defer cancel()
			model := &recordingModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: tc.parts(ctx, cancel)}, nil
			}}
			response := invocationHarness(t, limits, model).serve(request)
			require.Equal(t, http.StatusOK, response.Code)
			body := response.Body.String()
			overviews := documentOverviews(t, body, true)
			require.NotEmpty(t, overviews, body)
			assert.Equal(t, "claude-b", servedModels(overviews)[len(overviews)-1], body)
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
		}, 0},
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
			if tc.status != 0 {
				require.Equal(t, tc.status, response.Code, response.Body.String())
			} else {
				require.GreaterOrEqual(t, response.Code, http.StatusBadRequest, response.Body.String())
			}
			overviews := documentOverviews(t, response.Body.String(), false)
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
	assert.Equal(t, []string{"claude-b", "claude-c"}, servedModels(documentOverviews(t, response.Body.String(), true)))
}
