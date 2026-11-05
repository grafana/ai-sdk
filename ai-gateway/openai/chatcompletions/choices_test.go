package chatcompletions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nativeChoiceResult() *provider.GenerateResult {
	return &provider.GenerateResult{Choices: []provider.GenerateChoice{{Index: 1, Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "second"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonLength}}, {Index: 0, Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "first"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}}, Usage: textResult().Usage}
}
func indexedPart(index int, p provider.StreamPart) provider.StreamPart {
	p.ChoiceIndex = &index
	return p
}
func nativeChoiceParts() []provider.StreamPart {
	parts := []provider.StreamPart{{Type: provider.PartStreamStart}}
	for _, i := range []int{1, 0} {
		parts = append(parts, indexedPart(i, provider.StreamPart{Type: provider.PartTextStart, ID: "same"}))
	}
	for _, i := range []int{1, 0} {
		parts = append(parts, indexedPart(i, provider.StreamPart{Type: provider.PartTextDelta, ID: "same", Delta: fmt.Sprint(i)}), indexedPart(i, provider.StreamPart{Type: provider.PartTextEnd, ID: "same"}), indexedPart(i, provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}))
	}
	return append(parts, provider.StreamPart{Type: provider.PartChoicesFinish, Usage: &textResult().Usage})
}
func nativeChoiceHandler(t *testing.T, model provider.LanguageModel) *handler {
	t.Helper()
	c, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "canonical", Aliases: []string{"alias"}}, Model: model}})
	require.NoError(t, err)
	runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(c), Limits: execution.DefaultLimits})
	require.NoError(t, err)
	h, err := New(runtime)
	require.NoError(t, err)
	return h
}
func choiceRequest(stream bool) string {
	return fmt.Sprintf(`{"model":"alias","messages":[{"role":"user","content":"hi"}],"n":2,"stream":%t}`, stream)
}

func TestNativeChoices_Adapter(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			calls := 0
			f := &fakeModel{generate: func(_ context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
				calls++
				assert.Equal(t, 2, o.ChoiceCount)
				return nativeChoiceResult(), nil
			}, stream: func(_ context.Context, o provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				assert.Equal(t, 2, o.ChoiceCount)
				return partsStream(nativeChoiceParts()...), nil
			}}
			h := nativeChoiceHandler(t, execution.Translate(f, execution.Target{Type: execution.TargetOpenAICompatible}))
			request := httptest.NewRequest("POST", Path, strings.NewReader(choiceRequest(streaming)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			h.ServeHTTP(response, request)
			require.Equal(t, 200, response.Code, response.Body.String())
			assert.Equal(t, 1, calls)
			assert.NotContains(t, response.Body.String(), `"error":`)
			assert.Contains(t, response.Body.String(), `"index":0`)
			assert.Contains(t, response.Body.String(), `"index":1`)
			if streaming {
				assert.Contains(t, response.Body.String(), "[DONE]")
			} else {
				assert.Contains(t, response.Body.String(), "second")
			}
		})
	}
}

func TestNativeChoices_InvalidLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
	}{
		{"missing choice", []provider.StreamPart{indexedPart(0, provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}), {Type: provider.PartChoicesFinish, Usage: &provider.Usage{}}}},
		{"cross-choice text", []provider.StreamPart{indexedPart(0, provider.StreamPart{Type: provider.PartTextStart, ID: "text"}), indexedPart(1, provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "wrong"})}},
		{"missing aggregate finish", nativeChoiceParts()[:len(nativeChoiceParts())-1]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return partsStream(tc.parts...), nil
			}}
			h := nativeChoiceHandler(t, execution.Translate(f, execution.Target{Type: execution.TargetOpenAICompatible}))
			request := httptest.NewRequest("POST", Path, strings.NewReader(choiceRequest(true)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			h.ServeHTTP(response, request)
			assert.Contains(t, response.Body.String(), `"error":`)
			assert.NotContains(t, response.Body.String(), "[DONE]")
		})
	}
}

func TestNativeChoices_FallbackAndUnsupported(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			counts := [2]int{}
			failure := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "retry"})
			first := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				counts[0]++
				return nil, failure
			}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				counts[0]++
				return nil, failure
			}}
			second := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				counts[1]++
				return nativeChoiceResult(), nil
			}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				counts[1]++
				return partsStream(nativeChoiceParts()...), nil
			}}
			target := execution.Target{Type: execution.TargetOpenAICompatible}
			chain, err := fallback.New(execution.Translate(first, target), execution.Translate(second, target))
			require.NoError(t, err)
			h := nativeChoiceHandler(t, chain)
			request := httptest.NewRequest("POST", Path, strings.NewReader(choiceRequest(streaming)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			h.ServeHTTP(response, request)
			require.Equal(t, 200, response.Code, response.Body.String())
			assert.Equal(t, [2]int{1, 1}, counts)
			unsupported := testHandler(t, second)
			request = httptest.NewRequest("POST", Path, strings.NewReader(choiceRequest(streaming)))
			request.Header.Set("Content-Type", "application/json")
			response = httptest.NewRecorder()
			unsupported.ServeHTTP(response, request)
			assert.Equal(t, 400, response.Code)
			assert.Equal(t, [2]int{1, 1}, counts)
		})
	}
}

func TestNativeChoices_Cancel(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	f := &fakeModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		parts := make(chan provider.StreamPart, 1)
		parts <- provider.StreamPart{Type: provider.PartStreamStart}
		go func() { close(started); <-ctx.Done(); close(parts); close(stopped) }()
		return &provider.StreamResult{Stream: parts}, nil
	}}
	h := nativeChoiceHandler(t, execution.Translate(f, execution.Target{Type: execution.TargetOpenAICompatible}))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request := httptest.NewRequest("POST", Path, strings.NewReader(choiceRequest(true))).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { h.ServeHTTP(response, request); close(done) }()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not cancel")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("native stream did not cancel")
	}
	assert.NotContains(t, response.Body.String(), "[DONE]")
}

func TestNativeChoices_MetadataAndStructuredIsolation(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, invalid := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/invalid=%t", streaming, invalid), func(t *testing.T) {
				result := nativeChoiceResult()
				for i := range result.Choices {
					index := result.Choices[i].Index
					result.Choices[i].Content[0].Text = fmt.Sprintf(`{"choice":%d}`, index)
					result.Choices[i].FinishReason.Unified = provider.FinishReasonStop
					result.Choices[i].ProviderMetadata = provider.ProviderMetadata{"openaiCompatible": json.RawMessage(fmt.Sprintf(`{"chatCompletion":{"service_tier":"priority","system_fingerprint":"fp_choices","annotations":[{"type":"url_citation","url_citation":{"start_index":0,"end_index":1,"url":"https://example.com","title":"Example"}}],"audio":{"id":"audio-%d","data":"YQ==","expires_at":123,"transcript":"choice-%d"},"logprobs":{"content":[{"token":"choice-%d","logprob":-0.1,"bytes":null,"top_logprobs":[]}],"refusal":null}}}`, index, index, index))}
				}
				if invalid {
					result.Choices[0].Content[0].Text = "not JSON"
				}
				f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					parts := []provider.StreamPart{{Type: provider.PartStreamStart}}
					for _, choice := range result.Choices {
						for _, p := range []provider.StreamPart{{Type: provider.PartCustom, Kind: "openai-compatible.chat-completion", ProviderMetadata: choice.ProviderMetadata}, {Type: provider.PartTextStart, ID: "same"}, {Type: provider.PartTextDelta, ID: "same", Delta: choice.Content[0].Text}, {Type: provider.PartTextEnd, ID: "same"}, {Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}} {
							parts = append(parts, indexedPart(choice.Index, p))
						}
					}
					return partsStream(append(parts, provider.StreamPart{Type: provider.PartChoicesFinish, Usage: &result.Usage})...), nil
				}}
				h := nativeChoiceHandler(t, execution.Translate(f, execution.Target{Type: execution.TargetOpenAICompatible}))
				body := strings.TrimSuffix(choiceRequest(streaming), "}") + `,"response_format":{"type":"json_object"},"logprobs":true}`
				req := httptest.NewRequest("POST", Path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
				if invalid {
					assert.Contains(t, w.Body.String(), `"error":`)
					assert.NotContains(t, w.Body.String(), "[DONE]")
					return
				}
				require.NotContains(t, w.Body.String(), `"error":`)
				assert.Contains(t, w.Body.String(), `"service_tier":"priority"`)
				assert.Contains(t, w.Body.String(), `"system_fingerprint":"fp_choices"`)
				assert.Contains(t, w.Body.String(), `"annotations":[{"type":"url_citation"`)
				for _, i := range []int{0, 1} {
					assert.Contains(t, w.Body.String(), fmt.Sprintf(`"id":"audio-%d"`, i))
					assert.Contains(t, w.Body.String(), fmt.Sprintf(`"token":"choice-%d"`, i))
				}
				if streaming {
					assert.Contains(t, w.Body.String(), "[DONE]")
				}
			})
		}
	}
}
