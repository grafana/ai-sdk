package main

import (
	"context"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBufferedModelFallback_GenerateText(t *testing.T) {
	model, err := bufferedFallbackScenario()
	require.NoError(t, err)
	result, err := aisdk.GenerateText(context.Background(), model, aisdk.WithModelMessages(provider.UserText("decide")), aisdk.WithMaxRetries(0))
	require.NoError(t, err)
	assert.Equal(t, "Haiku decision", result.Text)
	assert.Equal(t, "anthropic", result.Response.Provider)
	assert.Equal(t, "claude-haiku-5-5", result.Response.ModelID)
	assert.Equal(t, intPtr(50), result.Usage.InputTokens.Total)
	require.Len(t, result.Steps, 1)
}
