package chatcompletions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeChoices_SourceDiagnosticsStayWithChoice(t *testing.T) {
	parts := nativeChoiceParts()
	source := provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "choice-zero-source", URL: "https://example.com"}}
	parts = append(parts[:1], append([]provider.StreamPart{indexedPart(0, source)}, parts[1:]...)...)
	f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return partsStream(parts...), nil
	}}
	h := nativeChoiceHandler(t, execution.Translate(f, execution.Target{Type: execution.TargetOpenAICompatible}))
	request := httptest.NewRequest("POST", Path, strings.NewReader(choiceRequest(true)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	require.Contains(t, response.Body.String(), "[DONE]")
	seen := false
	for _, line := range strings.Split(response.Body.String(), "\n") {
		if !strings.Contains(line, "choice-zero-source") {
			continue
		}
		var c chunk
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &c))
		require.Len(t, c.Choices, 1)
		assert.Equal(t, 0, c.Choices[0].Index, "source attribution crossed choices")
		seen = true
	}
	assert.True(t, seen)
}

func TestNativeChoices_FinalRefusal(t *testing.T) {
	for _, priorDelta := range []bool{false, true} {
		t.Run(fmt.Sprint(priorDelta), func(t *testing.T) {
			metadata := provider.ProviderMetadata{"openaiCompatible": json.RawMessage(`{"chatCompletion":{"refusal":"declined"}}`)}
			parts := []provider.StreamPart{{Type: provider.PartStreamStart}}
			for i := range 2 {
				if priorDelta {
					parts = append(parts, indexedPart(i, provider.StreamPart{Type: provider.PartCustom, Kind: "openai-compatible.chat-completion", ProviderMetadata: metadata}))
				}
				parts = append(parts, indexedPart(i, provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}, ProviderMetadata: metadata}))
			}
			parts = append(parts, provider.StreamPart{Type: provider.PartChoicesFinish, Usage: &provider.Usage{}})
			f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return partsStream(parts...), nil
			}}
			h := nativeChoiceHandler(t, execution.Translate(f, execution.Target{Type: execution.TargetOpenAICompatible}))
			body := strings.TrimSuffix(choiceRequest(true), "}") + `,"response_format":{"type":"json_object"},"tools":[{"type":"function","function":{"name":"f"}}],"tool_choice":"required"}`
			request := httptest.NewRequest("POST", Path, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			h.ServeHTTP(response, request)
			require.Contains(t, response.Body.String(), "[DONE]")
			assert.NotContains(t, response.Body.String(), `"error":`)
			refusals := map[int]string{}
			for _, line := range strings.Split(response.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data: {") {
					continue
				}
				var c chunk
				require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &c))
				for _, choice := range c.Choices {
					if choice.Delta.Refusal != nil {
						refusals[choice.Index] += *choice.Delta.Refusal
					}
				}
			}
			assert.Equal(t, map[int]string{0: "declined", 1: "declined"}, refusals)
		})
	}
}
