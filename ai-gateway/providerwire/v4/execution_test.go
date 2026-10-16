package v4

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executionTestCapture(count int, limit int64) *executionCapture {
	candidates := []catalog.ConfiguredCandidate{{ProviderInstance: "primary", Provider: "anthropic", ModelID: "model-a"}, {ProviderInstance: "secondary", Provider: "openai", ModelID: "model-b"}}
	_, capture := newExecutionContext(context.Background(), "alias", Selection{ID: "canonical", Candidates: candidates[:count]}, limit)
	return capture
}

func TestExecutionCapture(t *testing.T) {
	t.Run("ordered actual failures and native summary", func(t *testing.T) {
		capture := executionTestCapture(2, 8192)
		err := provider.NewAPICallError(provider.APICallErrorOptions{Message: "wrapper URL and raw body must not escape", StatusCode: 503, ResponseBody: `{"error":{"message":"native detail","type":"busy","code":9007199254740993},"private":"never"}`})
		capture.observe(context.Background(), fallback.Attempt{Index: 1, Provider: "anthropic", ModelID: "model-a", Outcome: fallback.AttemptFailed, WillFallback: true, Err: err, SourceErr: err})
		assert.Nil(t, capture.metadata(nil), "fallback intent is not a completed history")
		capture.observe(context.Background(), fallback.Attempt{Index: 2, Provider: "openai", ModelID: "model-b", Outcome: fallback.AttemptFailed, Err: err, SourceErr: err})
		raw := capture.metadata(nil)["gateway"]
		assert.Contains(t, string(raw), `"code":9007199254740993`)
		assert.NotContains(t, string(raw), "wrapper")
		assert.NotContains(t, string(raw), "private")
		assert.NotContains(t, string(raw), "willFallback")
		assert.JSONEq(t, `{"execution":{"requestedModelId":"alias","canonicalModelId":"canonical","attempts":[{"providerInstance":"primary","provider":"anthropic","modelId":"model-a","outcome":"failed","nativeError":{"message":"native detail","type":"busy","code":9007199254740993,"statusCode":503}},{"providerInstance":"secondary","provider":"openai","modelId":"model-b","outcome":"failed","nativeError":{"message":"native detail","type":"busy","code":9007199254740993,"statusCode":503}}]}}`, string(raw))
	})
	t.Run("canceled decision retains only owned native failure", func(t *testing.T) {
		failure := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Data: json.RawMessage(`{"message":"native detail","code":"busy"}`)})
		for _, sourceErr := range []error{failure, nil} {
			capture := executionTestCapture(1, 8192)
			capture.observe(context.Background(), fallback.Attempt{Index: 1, Provider: "anthropic", ModelID: "model-a", Outcome: fallback.AttemptCanceled, Err: context.Canceled, SourceErr: sourceErr})
			raw := string(capture.metadata(nil)["gateway"])
			assert.Contains(t, raw, `"outcome":"canceled"`)
			if sourceErr != nil {
				assert.Contains(t, raw, `"nativeError":{"message":"native detail","code":"busy","statusCode":503}`)
			} else {
				assert.NotContains(t, raw, `"nativeError"`)
			}
		}
	})
	t.Run("collision preserves native and sealing rejects late callbacks", func(t *testing.T) {
		capture := executionTestCapture(1, 8192)
		capture.direct(nil)
		capture.seal()
		capture.observe(context.Background(), fallback.Attempt{Index: 9})
		native := provider.ProviderMetadata{"gateway": json.RawMessage(`{"execution":{"opaque":true},"nativeMetadata":42}`), "other": json.RawMessage(`{"keep":true}`)}
		enriched := capture.metadata(native)
		assert.JSONEq(t, `{"execution":{"opaque":true},"nativeMetadata":42}`, string(native["gateway"]))
		var gateway struct {
			Native    json.RawMessage   `json:"nativeMetadata"`
			Execution executionOverview `json:"execution"`
		}
		require.NoError(t, json.Unmarshal(enriched["gateway"], &gateway))
		assert.Equal(t, native["gateway"], gateway.Native)
		require.Len(t, gateway.Execution.Attempts, 1)
		assert.Equal(t, native["other"], enriched["other"])
	})
	t.Run("inconsistent history omitted", func(t *testing.T) {
		for _, attempt := range []fallback.Attempt{{Index: 2, Provider: "openai", ModelID: "model-b", Outcome: fallback.AttemptFailed}, {Index: 1, Provider: "wrong", ModelID: "model-a", Outcome: fallback.AttemptSelected}} {
			capture := executionTestCapture(2, 8192)
			capture.observe(context.Background(), attempt)
			assert.Nil(t, capture.metadata(nil))
		}
	})
	t.Run("concurrent request isolation", func(t *testing.T) {
		var wait sync.WaitGroup
		for range 32 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				capture := executionTestCapture(1, 8192)
				capture.direct(nil)
				capture.seal()
				assert.Contains(t, string(capture.metadata(nil)["gateway"]), `"outcome":"selected"`)
			}()
		}
		wait.Wait()
	})
}

func TestExecutionEnrichment_ExistingEnvelopeLimits(t *testing.T) {
	capture := executionTestCapture(1, 8192)
	capture.direct(nil)
	h := &handler{limits: testLimits()}
	h.limits.UnaryResponseBytes = int64(len(canonicalUpstreamError))
	response := httptest.NewRecorder()
	h.writeExecutionError(response, safeError{category: safeUpstream}, capture)
	assert.Equal(t, string(canonicalUpstreamError), response.Body.String())
	h.limits.StreamFrameBytes = int64(len(canonicalUpstreamStreamErrorFrame))
	response = httptest.NewRecorder()
	assert.Equal(t, streamWriteSuccess, h.emitExecutionStreamError(response, safeError{category: safeUpstream}, capture, nil))
	assert.Equal(t, string(canonicalUpstreamStreamErrorFrame), response.Body.String())
	h.limits.StreamFrameBytes = 8192
	response = httptest.NewRecorder()
	api := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 400, Data: json.RawMessage(`{"message":"event local","code":"invalid_schema"}`)})
	assert.Equal(t, streamWriteSuccess, h.emitExecutionStreamError(response, safeError{category: safeFailedDependency}, capture, api))
	frame := response.Body.Bytes()
	require.NoError(t, compileWireSchema(t, streamEventSchemaJSON).Validate(frame[6:len(frame)-2]))
	assert.Contains(t, string(frame), `"nativeError":{"message":"event local","code":"invalid_schema","statusCode":400}`)
	assert.Contains(t, string(frame), `"outcome":"selected"`)
}

func TestNativeErrorSummary(t *testing.T) {
	for _, data := range []string{`not JSON`, `null`, `{"code":{}}`, `{"code":true}`, `{"code":null}`} {
		api := provider.NewAPICallError(provider.APICallErrorOptions{Message: "untrusted wrapper", StatusCode: 400, Data: json.RawMessage(data)})
		summary := summarizeNativeError(api, 1024)
		require.NotNil(t, summary)
		assert.Empty(t, summary.Message)
		assert.Empty(t, summary.Code)
	}
	api := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 400, ResponseBody: strings.Repeat("x", 2048)})
	assert.Equal(t, &nativeErrorSummary{StatusCode: 400}, summarizeNativeError(api, 1024))
}

func TestExecutionError_DefaultClientEnvelopeLimit(t *testing.T) {
	capture := executionTestCapture(1, 1<<20)
	raw, _ := json.Marshal(map[string]string{"message": strings.Repeat("x", 70<<10)})
	api := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Data: raw})
	capture.direct(api)
	h := &handler{limits: testLimits()}
	h.limits.UnaryResponseBytes = 1 << 20
	response := httptest.NewRecorder()
	h.writeExecutionError(response, safeError{category: safeOverload}, capture)
	assert.Equal(t, string(canonicalOverloadError), response.Body.String())
}

func TestExecutionEnrichment_SuccessAndAdaptation(t *testing.T) {
	capture := executionTestCapture(1, 8192)
	capture.direct(nil)
	h := &handler{limits: testLimits()}
	result := validGenerateResult()
	original := httptest.NewRecorder()
	require.True(t, h.writeUnarySuccess(original, result))
	h.limits.UnaryResponseBytes = int64(original.Body.Len())
	limited := httptest.NewRecorder()
	require.True(t, h.writeUnarySuccess(limited, result, unaryMappingContext{execution: capture}))
	assert.Equal(t, original.Body.String(), limited.Body.String())
	h.limits.UnaryResponseBytes = 8192
	result.ProviderMetadata = provider.ProviderMetadata{"gateway": json.RawMessage(`not-json`)}
	require.False(t, h.writeUnarySuccess(httptest.NewRecorder(), result, unaryMappingContext{execution: capture}), "enrichment cannot repair invalid native metadata")
	response := httptest.NewRecorder()
	state := newStreamState(10, nil)
	state.execution = capture
	adaptation := h.processStreamPart(response, state, provider.StreamPart{Type: provider.PartTextDelta, ID: "not-open", Delta: "invalid"})
	require.Equal(t, streamPartAdapterFailure, adaptation)
	canceled := false
	require.True(t, h.handleStreamPartResult(response, func() { canceled = true }, adaptation, capture))
	assert.True(t, canceled)
	assert.Contains(t, response.Body.String(), `"outcome":"selected"`)
	assert.NotContains(t, response.Body.String(), `"nativeError"`)
	assert.Contains(t, response.Body.String(), `"code":"internal_error"`)
}

func TestExecutionCapture_ConcurrentSealedSnapshot(t *testing.T) {
	capture := executionTestCapture(1, 8192)
	capture.direct(nil)
	expected := string(capture.metadata(nil)["gateway"])
	capture.seal()
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			capture.observe(context.Background(), fallback.Attempt{Index: 2, Provider: "late", ModelID: "late", Outcome: fallback.AttemptFailed})
			capture.seal()
			assert.Equal(t, expected, string(capture.metadata(nil)["gateway"]))
		}()
	}
	wait.Wait()
}
