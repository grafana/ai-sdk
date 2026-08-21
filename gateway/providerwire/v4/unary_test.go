package v4

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeUnary_GoldenAndPrivacy(t *testing.T) {
	three, two := 3, 2
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "first", ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":true}`)}},
			{Type: provider.ContentText, Text: ""},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "backend-stop"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: &three},
			OutputTokens: provider.OutputTokenUsage{Total: &two, Text: &two},
			Raw:          json.RawMessage(`{"private":true}`),
		},
		ProviderMetadata: provider.ProviderMetadata{"private-provider": json.RawMessage(`{"secret":true}`)},
		Request:          &provider.RequestMetadata{Body: json.RawMessage(`{"secret":true}`)},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{ModelID: "backend-model", Provider: "private-provider"},
			Headers:          map[string]string{"Authorization": "secret"},
			Body:             json.RawMessage(`{"secret":true}`),
		},
	}
	handler := newTestHandler(t, &testModel{})
	body, err := handler.encodeUnary(result, "public/canonical")
	require.NoError(t, err)
	assert.Equal(t, `{"content":[{"type":"text","text":"first"},{"type":"text","text":""}],"finishReason":{"unified":"stop","raw":"backend-stop"},"usage":{"inputTokens":{"total":3},"outputTokens":{"total":2,"text":2}},"warnings":[],"response":{"modelId":"public/canonical"}}`, string(body))
	require.NoError(t, handler.schemas.unary.Validate(body))
	for _, private := range []string{"secret", "backend-model", "private-provider", "Authorization"} {
		assert.NotContains(t, string(body), private)
	}
}

func TestMapFinishReason_SupportsRegisteredVocabulary(t *testing.T) {
	for _, reason := range []provider.UnifiedFinishReason{
		provider.FinishReasonStop,
		provider.FinishReasonLength,
		provider.FinishReasonContentFilter,
		provider.FinishReasonToolCalls,
		provider.FinishReasonError,
		provider.FinishReasonOther,
	} {
		mapped, err := mapFinishReason(provider.FinishReason{Unified: reason})
		require.NoError(t, err)
		assert.Equal(t, reason, mapped.Unified)
	}
}

func TestEncodeUnary_RejectsInvalidUTF8(t *testing.T) {
	invalid := string([]byte{0xff})
	handler := newTestHandler(t, &testModel{})
	tests := []struct {
		name    string
		result  *provider.GenerateResult
		modelID string
	}{
		{"text", func() *provider.GenerateResult {
			result := validGenerateResult()
			result.Content[0].Text = invalid
			return result
		}(), "public/canonical"},
		{"raw finish reason", func() *provider.GenerateResult {
			result := validGenerateResult()
			result.FinishReason.Raw = invalid
			return result
		}(), "public/canonical"},
		{"canonical model ID", validGenerateResult(), invalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handler.encodeUnary(tc.result, tc.modelID)
			assert.Error(t, err)
		})
	}
}

func TestMapUsage_AllCountersAndSafeIntegerBoundary(t *testing.T) {
	values := []int{1, 2, 3, 4, 5, 6, 7}
	usage := provider.Usage{
		InputTokens:  provider.InputTokenUsage{Total: &values[0], NoCache: &values[1], CacheRead: &values[2], CacheWrite: &values[3]},
		OutputTokens: provider.OutputTokenUsage{Total: &values[4], Text: &values[5], Reasoning: &values[6]},
	}
	mapped, err := mapUsage(usage)
	require.NoError(t, err)
	body, err := json.Marshal(mapped)
	require.NoError(t, err)
	assert.Equal(t, `{"inputTokens":{"total":1,"noCache":2,"cacheRead":3,"cacheWrite":4},"outputTokens":{"total":5,"text":6,"reasoning":7}}`, string(body))

	if int64(^uint(0)>>1) > maxSafeInteger {
		unsafe := int(maxSafeInteger + 1)
		usage.InputTokens.Total = &unsafe
		_, err = mapUsage(usage)
		assert.Error(t, err)
	}
}

func TestEncodeUnary_RejectsInvalidResults(t *testing.T) {
	one := 1
	tests := []struct {
		name   string
		result *provider.GenerateResult
	}{
		{"nil", nil},
		{"warnings", func() *provider.GenerateResult {
			r := validGenerateResult()
			r.Warnings = []provider.Warning{{Type: provider.WarnOther}}
			return r
		}()},
		{"unsupported content", func() *provider.GenerateResult {
			r := validGenerateResult()
			r.Content[0].Type = provider.ContentReasoning
			return r
		}()},
		{"invalid text fields", func() *provider.GenerateResult {
			r := validGenerateResult()
			r.Content[0].ToolName = "private"
			return r
		}()},
		{"invalid finish", func() *provider.GenerateResult {
			r := validGenerateResult()
			r.FinishReason.Unified = "invalid"
			return r
		}()},
		{"negative usage", func() *provider.GenerateResult {
			r := validGenerateResult()
			negative := -1
			r.Usage.InputTokens.Total = &negative
			return r
		}()},
		{"valid optional usage", &provider.GenerateResult{Content: []provider.GenerateContentPart{}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther}, Usage: provider.Usage{InputTokens: provider.InputTokenUsage{Total: &one}}}},
	}
	handler := newTestHandler(t, &testModel{})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handler.encodeUnary(tc.result, "public/canonical")
			if tc.name == "valid optional usage" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestHandler_UnaryDispatchAndLimit(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		model := &testModel{}
		handler := newTestHandler(t, model)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, false))
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, MIMEJSON, response.Header().Get("Content-Type"))
		assert.Equal(t, 1, model.calls)
		assert.Contains(t, response.Body.String(), `"warnings":[]`)
		assert.Contains(t, response.Body.String(), `"modelId":"public/canonical"`)
	})

	t.Run("output limit before 200", func(t *testing.T) {
		model := &testModel{}
		handler := newTestHandler(t, model, WithMaxUnaryResponseBytes(1))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, false))
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, 1, model.calls)
	})
}

func TestHandler_UnaryInvocationIsBoundedWhenModelIgnoresContext(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	model := &testModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		close(started)
		<-release
		return validGenerateResult(), nil
	}}
	handler := newTestHandler(t, model, WithTotalTimeout(20*time.Millisecond))
	response := httptest.NewRecorder()
	start := time.Now()
	handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, false))
	assert.Less(t, time.Since(start), time.Second)
	assert.Equal(t, http.StatusGatewayTimeout, response.Code)
	assert.Equal(t, MIMEJSON, response.Header().Get("Content-Type"))
	<-started
	close(release)
}

func TestHandler_UnaryCancellationWinsReadyResult(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	model := &testModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		close(started)
		<-release
		return validGenerateResult(), nil
	}}
	handler := newTestHandler(t, model)
	ctx, cancel := context.WithCancel(context.Background())
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, false).WithContext(ctx))
		close(done)
	}()
	<-started
	cancel()
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not return after cancellation")
	}
	assert.Equal(t, 499, response.Code)
}

func TestHandler_UnaryProviderErrorsAndTimeout(t *testing.T) {
	t.Run("permanent provider error", func(t *testing.T) {
		retryable := false
		model := &testModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, provider.NewAPICallError(provider.APICallErrorOptions{
				Message:           "private-message",
				URL:               "https://private-url",
				RequestBodyValues: json.RawMessage(`{"privateRequest":true}`),
				ResponseHeaders:   map[string][]string{"X-Private": {"private-header"}},
				ResponseBody:      "private-body",
				StatusCode:        401,
				IsRetryable:       &retryable,
				Data:              json.RawMessage(`{"privateData":true}`),
				Cause:             errors.New("private-cause"),
			})
		}}
		handler := newTestHandler(t, model)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, false))
		assert.Equal(t, http.StatusFailedDependency, response.Code)
		assert.Equal(t, MIMEJSON, response.Header().Get("Content-Type"))
		assert.Equal(t, `{"error":{"message":"provider request failed","type":"failed_dependency","param":null,"code":"failed_dependency"}}`, response.Body.String())
		assert.NotContains(t, response.Body.String(), "private")
	})

	t.Run("total timeout", func(t *testing.T) {
		model := &testModel{generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		handler := newTestHandler(t, model, WithTotalTimeout(10*time.Millisecond))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, false))
		assert.Equal(t, http.StatusGatewayTimeout, response.Code)
	})

	t.Run("request cancellation", func(t *testing.T) {
		model := &testModel{generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		handler := newTestHandler(t, model)
		ctx, cancel := context.WithCancel(context.Background())
		request := validRequest(`{"prompt":[]}`, false).WithContext(ctx)
		cancel()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assert.Equal(t, 499, response.Code)
	})
}
