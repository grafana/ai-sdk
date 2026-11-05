package openaicompatible

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeChoices_Unary(t *testing.T) {
	body := `{"id":"native","model":"model","service_tier":"priority","system_fingerprint":"fp_choices","choices":[{"index":1,"message":{"role":"assistant","content":"second"},"finish_reason":"length"},{"index":0,"message":{"role":"assistant","content":null,"refusal":"declined"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":4,"total_tokens":6}}`
	result, err := parseGenerateResponse([]byte(body), nil, "test.chat", "test", func() string { return "id" }, true, 2)
	require.NoError(t, err)
	require.Len(t, result.Choices, 2)
	assert.Equal(t, 1, result.Choices[0].Index)
	assert.Equal(t, "second", result.Choices[0].Content[0].Text)
	assert.Equal(t, provider.FinishReasonLength, result.Choices[0].FinishReason.Unified)
	assert.Contains(t, string(result.ProviderMetadata["openaiCompatible"]), `"system_fingerprint":"fp_choices"`)
	assert.Empty(t, result.Content)
	assert.Empty(t, result.FinishReason.Unified)
	assert.Equal(t, 2, *result.Usage.InputTokens.Total)
	assert.Equal(t, 4, *result.Usage.OutputTokens.Total)
	assert.Contains(t, string(result.Choices[1].ProviderMetadata["openaiCompatible"]), "declined")
	for _, bad := range []string{strings.Replace(body, `"index":1`, `"index":0`, 1), strings.Replace(body, `"index":1`, `"index":2`, 1), strings.Replace(body, `"index":1,`, "", 1)} {
		_, err := parseGenerateResponse([]byte(bad), nil, "test", "test", func() string { return "id" }, true, 2)
		require.Error(t, err)
	}
	single, err := parseGenerateResponse([]byte(body), nil, "test", "test", func() string { return "id" }, false)
	require.NoError(t, err)
	assert.Nil(t, single.Choices)
	assert.Equal(t, "second", single.Content[0].Text)
}

func TestNativeChoices_Stream(t *testing.T) {
	frames := []string{
		`{"id":"native","choices":[{"index":1,"delta":{"content":"B"}},{"index":0,"delta":{"content":"A"}}]}`,
		`{"choices":[{"index":1,"delta":{},"finish_reason":"length"}]}`,
		`{"choices":[{"index":0,"delta":{"content":"a"},"finish_reason":"stop"}]}`,
		`{"choices":[],"service_tier":"priority","system_fingerprint":"fp_final","usage":{"prompt_tokens":2,"completion_tokens":4,"total_tokens":6}}`,
	}
	body := ""
	for _, frame := range frames {
		body += "data: " + frame + "\n\n"
	}
	body += "data: [DONE]\n\n"
	out := make(chan provider.StreamPart, 40)
	(&model{providerName: "test.chat"}).runStream(t.Context(), "https://test", nil, strings.NewReader(body), nil, nil, false, "test", out, true, 2)
	close(out)
	text := map[int]string{}
	finishes := map[int]provider.UnifiedFinishReason{}
	totals := 0
	for p := range out {
		require.NotEqual(t, provider.PartError, p.Type)
		if p.Type == provider.PartTextDelta {
			require.NotNil(t, p.ChoiceIndex)
			text[*p.ChoiceIndex] += p.Delta
		}
		if p.Type == provider.PartFinish {
			require.NotNil(t, p.ChoiceIndex)
			finishes[*p.ChoiceIndex] = p.FinishReason.Unified
			assert.Nil(t, p.Usage.InputTokens.Total)
		}
		if p.Type == provider.PartChoicesFinish {
			totals++
			assert.Contains(t, string(p.ProviderMetadata["openaiCompatible"]), `"system_fingerprint":"fp_final"`)
			assert.Nil(t, p.ChoiceIndex)
			assert.Nil(t, p.FinishReason)
			assert.Equal(t, 2, *p.Usage.InputTokens.Total)
			assert.Equal(t, 4, *p.Usage.OutputTokens.Total)
		}
	}
	assert.Equal(t, map[int]string{0: "Aa", 1: "B"}, text)
	assert.Equal(t, map[int]provider.UnifiedFinishReason{0: provider.FinishReasonStop, 1: provider.FinishReasonLength}, finishes)
	assert.Equal(t, 1, totals)
	for _, bad := range []string{strings.Replace(body, `"index":1`, `"index":2`, 1), strings.Replace(body, `"index":1,`, "", 1), "data: " + frames[0] + "\n\ndata: [DONE]\n\n"} {
		out := make(chan provider.StreamPart, 40)
		(&model{providerName: "test"}).runStream(t.Context(), "https://test", nil, strings.NewReader(bad), nil, nil, false, "test", out, false, 2)
		close(out)
		failed := false
		for p := range out {
			if p.Type == provider.PartError {
				failed = true
			}
			assert.NotEqual(t, provider.PartChoicesFinish, p.Type)
		}
		assert.True(t, failed)
	}
}

func TestNativeChoices_Request(t *testing.T) {
	m := &model{modelID: "test", providerName: "test"}
	body, _, err := m.buildRequest(provider.CallOptions{ChoiceCount: 2, ProviderOptions: provider.ProviderOptions{"test": provider.RawProviderOption{Key: "test", Raw: json.RawMessage(`{"n":5}`)}}}, true)
	require.NoError(t, err)
	assert.Equal(t, 2, body["n"])
	body, _, err = m.buildRequest(provider.CallOptions{}, false)
	require.NoError(t, err)
	assert.NotContains(t, body, "n")
}
