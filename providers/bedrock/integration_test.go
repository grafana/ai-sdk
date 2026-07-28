package bedrock

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

// TestE2EBedrock_TextGeneration calls the real Bedrock Converse API. Skipped
// unless `BEDROCK_E2E=1` and AWS credentials are available in the
// environment (env vars or shared config). Use BEDROCK_E2E_MODEL_ID and
// BEDROCK_E2E_REGION to override defaults.
//
// Cost-minimised: short prompt, low max-tokens, no streaming.
func TestE2EBedrock_TextGeneration(t *testing.T) {
	if os.Getenv("BEDROCK_E2E") != "1" {
		t.Skip("set BEDROCK_E2E=1 to run end-to-end tests against the real Bedrock API")
	}
	modelID := envOrDefault("BEDROCK_E2E_MODEL_ID", "anthropic.claude-haiku-4-5-20251001-v1:0")
	region := envOrDefault("BEDROCK_E2E_REGION", "us-east-1")

	lm := New(modelID, WithRegion(region))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	maxTok := 32
	temp := 0.0
	result, err := lm.DoGenerate(ctx, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("Reply with the single word 'pong'.")},
		MaxOutputTokens: &maxTok,
		Temperature:     &temp,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)
}

// TestE2EBedrock_StreamingTextGeneration exercises the converse-stream path
// against the real Bedrock API. Skipped unless BEDROCK_E2E=1.
func TestE2EBedrock_StreamingTextGeneration(t *testing.T) {
	if os.Getenv("BEDROCK_E2E") != "1" {
		t.Skip("set BEDROCK_E2E=1 to run end-to-end tests against the real Bedrock API")
	}
	modelID := envOrDefault("BEDROCK_E2E_MODEL_ID", "anthropic.claude-haiku-4-5-20251001-v1:0")
	region := envOrDefault("BEDROCK_E2E_REGION", "us-east-1")

	lm := New(modelID, WithRegion(region))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	maxTok := 32
	result, err := lm.DoStream(ctx, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("Reply with the single word 'pong'.")},
		MaxOutputTokens: &maxTok,
	})
	require.NoError(t, err)

	var finished bool
	for part := range result.Stream {
		if part.Type == provider.PartFinish {
			finished = true
		}
	}
	require.True(t, finished, "stream did not produce a finish part")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
