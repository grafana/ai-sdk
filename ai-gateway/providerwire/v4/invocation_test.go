package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func invocationHarness(t *testing.T, limits Limits, models ...*recordingModel) *runtimeHarness {
	t.Helper()
	h := newRuntimeHarness(t, limits)
	candidates := make([]provider.LanguageModel, len(models))
	for i, model := range models {
		candidates[i] = model
		h.resolver.resolved.Candidates = append(h.resolver.resolved.Candidates, catalog.ConfiguredCandidate{Provider: "native", ProviderInstance: "configured", ModelID: string(rune('A' + i))})
	}
	h.resolver.resolved.Model = candidates[0]
	if len(candidates) > 1 {
		ordered, err := fallback.New(candidates...)
		require.NoError(t, err)
		ordered.WithAttemptObserver(func(context.Context, fallback.Attempt) { panic("operator panic") })
		h.resolver.resolved.Model = ordered
	}
	return h
}

func readOverview(t *testing.T, body []byte) *execution.Overview {
	t.Helper()
	var value struct {
		Metadata map[string]struct {
			Overview *execution.Overview `json:"execution"`
		} `json:"providerMetadata"`
	}
	require.NoError(t, json.Unmarshal(body, &value))
	return value.Metadata["gateway"].Overview
}

func TestInvocation_Unary(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		primary, secondary       int
		wantStatus, wantAttempts int
	}{
		{name: "direct", wantStatus: 200, wantAttempts: 1},
		{name: "secondary", primary: 503, wantStatus: 200, wantAttempts: 2},
		{name: "noneligible", primary: 401, wantStatus: 424, wantAttempts: 1},
		{name: "exhausted", primary: 503, secondary: 400, wantStatus: 424, wantAttempts: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				if tc.primary != 0 {
					return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "first failure", StatusCode: tc.primary})
				}
				return validGenerateResult(), nil
			}}
			models := []*recordingModel{first}
			if tc.name != "direct" {
				models = append(models, &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					if tc.secondary != 0 {
						return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "second failure", StatusCode: tc.secondary})
					}
					return validGenerateResult(), nil
				}})
			}
			h := invocationHarness(t, testLimits(), models...)
			response := h.serve(validRequest(`{"prompt":[]}`))
			require.Equal(t, tc.wantStatus, response.Code)
			overview := readOverview(t, response.Body.Bytes())
			require.NotNil(t, overview)
			require.Len(t, overview.Attempts, tc.wantAttempts)
			assert.Equal(t, "A", overview.Attempts[0].ModelID)
			if tc.primary != 0 {
				require.NotNil(t, overview.Attempts[0].Error)
				assert.Equal(t, "first failure", overview.Attempts[0].Error.Message)
			}
			assert.Equal(t, 1, first.callCount())
			if len(models) == 2 {
				assert.Equal(t, tc.wantAttempts-1, models[1].callCount())
			}
		})
	}
}

func TestInvocation_StreamCurrentErrors(t *testing.T) {
	first := &recordingModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "current failure", StatusCode: 503})},
			provider.StreamPart{Type: provider.PartTextStart, ID: "text"},
			provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "after"},
			provider.StreamPart{Type: provider.PartTextEnd, ID: "text"}, finishPart())}, nil
	}}
	second := &recordingModel{}
	h := invocationHarness(t, testLimits(), first, second)
	response := h.serve(streamRequest(`{"prompt":[]}`))
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, `"nativeError":{"message":"current failure"`)
	assert.Contains(t, body, `"outcome":"selected"`)
	assert.Less(t, strings.Index(body, `"type":"error"`), strings.Index(body, `"type":"text-delta"`))
	finish := body[strings.LastIndex(body, "data: "):]
	assert.NotContains(t, finish, "current failure")
	assert.NotContains(t, body, "selectedAttempt")
	assert.NotContains(t, body, "completion")
	assert.Zero(t, second.callCount())
}

func TestInvocation_OptionalLimitsAndNamespace(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "unary", true: "finish"}[streaming], func(t *testing.T) {
			native := provider.ProviderMetadata{"gateway": json.RawMessage(`{"opaque":"native namespace"}`)}
			first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				result := validGenerateResult()
				result.ProviderMetadata = native
				return result, nil
			}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				finish := finishPart()
				finish.ProviderMetadata = native
				return &provider.StreamResult{Stream: makeStream(finish)}, nil
			}}
			baseline := newRuntimeHarness(t, testLimits())
			baseline.resolver.resolved.Model = first
			request := func() *http.Request {
				if streaming {
					return streamRequest(`{"prompt":[]}`)
				}
				return validRequest(`{"prompt":[]}`)
			}
			original := baseline.serve(request()).Body.Bytes()
			full := invocationHarness(t, testLimits(), first).serve(request()).Body.Bytes()
			size := len(full)
			if streaming {
				frames := strings.Split(strings.TrimSuffix(string(full), "\n\n"), "\n\n")
				size = len(frames[len(frames)-1]) + 2
			}
			for _, delta := range []int{0, -1} {
				limits := testLimits()
				if streaming {
					limits.StreamFrameBytes = int64(size + delta)
				} else {
					limits.UnaryResponseBytes = int64(size + delta)
				}
				response := invocationHarness(t, limits, first).serve(request())
				assert.Equal(t, 200, response.Code)
				if delta == 0 {
					assert.Equal(t, full, response.Body.Bytes())
					assert.Contains(t, response.Body.String(), `"nativeMetadata":{"opaque":"native namespace"}`)
				} else {
					assert.Equal(t, original, response.Body.Bytes())
				}
			}
			assert.JSONEq(t, `{"opaque":"native namespace"}`, string(native["gateway"]))
		})
	}
	first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		result := validGenerateResult()
		result.ProviderMetadata = provider.ProviderMetadata{"gateway": json.RawMessage(`true`)}
		return result, nil
	}}
	response := invocationHarness(t, testLimits(), first).serve(validRequest(`{"prompt":[]}`))
	assert.Equal(t, 500, response.Code)
	assert.NotContains(t, response.Body.String(), "nativeMetadata")
}

func TestInvocation_PrimaryEncodingFailure(t *testing.T) {
	first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		result := validGenerateResult()
		result.Content = []provider.GenerateContentPart{{Type: provider.ContentText, Text: strings.Repeat("<", 80)}}
		return result, nil
	}}
	limits := testLimits()
	limits.UnaryResponseBytes = 400
	response := invocationHarness(t, limits, first).serve(validRequest(`{"prompt":[]}`))
	assert.Equal(t, 500, response.Code)
	assert.NotContains(t, response.Body.String(), `"content"`)
	finish := finishPart()
	finish.FinishReason.Raw = strings.Repeat("<", 80)
	first.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(finish)}, nil
	}
	limits.StreamFrameBytes = 400
	response = invocationHarness(t, limits, first).serve(streamRequest(`{"prompt":[]}`))
	assert.Contains(t, response.Body.String(), `"code":"internal_error"`)
	assert.NotContains(t, response.Body.String(), `"type":"finish"`)
}

func TestInvocation_PrivateSourcesAndErrorLimit(t *testing.T) {
	first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "native rejected actual-source", StatusCode: 401})
	}}
	h := invocationHarness(t, testLimits(), first)
	h.resolver.resolved.ProtectedSources = []string{"actual-source"}
	response := h.serve(validRequest(`{"prompt":[]}`))
	assert.Equal(t, 424, response.Code)
	assert.NotContains(t, response.Body.String(), "actual-source")
	overview := readOverview(t, response.Body.Bytes())
	require.NotNil(t, overview)
	assert.Equal(t, 401, overview.Attempts[0].Error.StatusCode)
	first.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: strings.Repeat("x", maxErrorResponseBytes), StatusCode: 401})
	}
	response = h.serve(validRequest(`{"prompt":[]}`))
	assert.Equal(t, 424, response.Code)
	assert.Equal(t, canonicalDependencyError, response.Body.Bytes())
}

func TestInvocation_RequestCredentialEcho(t *testing.T) {
	for _, request := range []string{
		`{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":[{"authorizationToken":"remote-credential"}]}}}`,
		`{"prompt":[],"tools":[{"type":"provider","id":"openai.mcp","name":"mcp","args":{"authorization":"remote-credential"}}]}`,
		`{"prompt":[],"tools":[{"type":"provider","id":"openai.mcp","name":"mcp","args":{"headers":{"authorization":"Bearer remote-credential"}}}]}`,
	} {
		t.Run(request, func(t *testing.T) {
			first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "provider echoed remote-credential", StatusCode: 401})
			}}
			response := invocationHarness(t, testLimits(), first).serve(validRequest(request))
			assert.Equal(t, 424, response.Code)
			assert.NotContains(t, response.Body.String(), "remote-credential")
			require.NotNil(t, readOverview(t, response.Body.Bytes()))
			assert.Equal(t, 401, readOverview(t, response.Body.Bytes()).Attempts[0].Error.StatusCode)
		})
	}
}

func TestInvocation_CanceledLateDirectSetup(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	first := &recordingModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		close(entered)
		<-release
		defer close(finished)
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "unowned late native failure", StatusCode: 401})
	}}
	h := invocationHarness(t, testLimits(), first)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	responses := make(chan string, 1)
	go func() { responses <- h.serve(streamRequest(`{"prompt":[]}`).WithContext(ctx)).Body.String() }()
	<-entered
	cancel()
	var body string
	select {
	case body = <-responses:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not return")
	}
	assert.Contains(t, body, `"outcome":"canceled"`)
	assert.NotContains(t, body, "unowned late native failure")
	close(release)
	<-finished
	assert.NotContains(t, body, "unowned late native failure")
}

func TestInvocation_SharedModelIsolation(t *testing.T) {
	first := &recordingModel{generate: func(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
		result := validGenerateResult()
		result.ProviderMetadata = provider.ProviderMetadata{"native": json.RawMessage(`{"id":"` + options.Headers["request-id"] + `"}`)}
		return result, nil
	}}
	h := invocationHarness(t, testLimits(), first)
	var group sync.WaitGroup
	for i := range 24 {
		group.Go(func() {
			id := string(rune('A' + i))
			request := validRequest(`{"prompt":[],"headers":{"request-id":"` + id + `"}}`)
			response := h.serve(request)
			require.Equal(t, 200, response.Code)
			require.Len(t, readOverview(t, response.Body.Bytes()).Attempts, 1)
			assert.Contains(t, response.Body.String(), `"native":{"id":"`+id+`"}`)
		})
	}
	group.Wait()
	assert.Equal(t, 24, first.callCount())
}
