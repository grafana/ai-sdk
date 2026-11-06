package chatcompletions

import (
	"encoding/json"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExtendedUsageProjection(t *testing.T) {
	native := &compatible.ChatCompletionUsage{PromptTokensDetails: &compatible.ChatCompletionPromptTokensDetails{CacheWriteTokens: ptr(0), ImageTokens: ptr(2), TextTokens: ptr(3)}, CompletionTokensDetails: &compatible.ChatCompletionOutputTokensDetails{TextTokens: ptr(0)}}
	u := &usage{Prompt: 5, Completion: 1, Total: 6}
	require.NoError(t, nativeUsage(u, native))
	raw, err := json.Marshal(u)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6,"prompt_tokens_details":{"cache_write_tokens":0,"image_tokens":2,"text_tokens":3},"completion_tokens_details":{"text_tokens":0}}`, string(raw))
	for _, bad := range []int{-1, 6} {
		native.PromptTokensDetails.ImageTokens = ptr(bad)
		require.Error(t, nativeUsage(&usage{Prompt: 5, Completion: 1, Total: 6}, native))
	}
	native.PromptTokensDetails = nil
	native.CompletionTokensDetails.TextTokens = ptr(2)
	require.Error(t, nativeUsage(&usage{Prompt: 5, Completion: 1, Total: 6}, native))
	missing := &usage{Prompt: 5, Completion: 1, Total: 6}
	require.NoError(t, nativeUsage(missing, &compatible.ChatCompletionUsage{}))
	assert.Nil(t, missing.PromptDetails)
	assert.Nil(t, missing.CompletionDetails)
}
