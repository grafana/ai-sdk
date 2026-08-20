//go:build conformance

package bedrock_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	providerwirev4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	bedrockProvider "github.com/grafana/ai-sdk/providers/bedrock"
	"github.com/grafana/ai-sdk/test/conformance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingUnaryModel struct {
	provider.LanguageModel
	generateCalls atomic.Int32
	streamCalls   atomic.Int32
}

func (m *countingUnaryModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	m.generateCalls.Add(1)
	return m.LanguageModel.DoGenerate(ctx, options)
}
func (m *countingUnaryModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	m.streamCalls.Add(1)
	return m.LanguageModel.DoStream(ctx, options)
}

type normalizedUnaryArtifact struct {
	Result json.RawMessage `json:"result"`
}

func TestProviderWireV4_JSONToolWithAnswer(t *testing.T) {
	fixtureDir := filepath.Join("upstream", "json-tool-with-answer")
	cfg, err := conformance.LoadConfig(filepath.Join(fixtureDir, "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, conformance.OperationGenerate, cfg.Operation)

	replay, err := conformance.NewReplayServerWithFraming(fixtureDir, "bedrock", conformance.BedrockFraming{})
	require.NoError(t, err)
	replay.SetGenerateResponseDate(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	t.Cleanup(replay.Close)

	creds := credentials.NewStaticCredentialsProvider("AKID-test", "secret-test", "")
	realModel := bedrockProvider.New(
		cfg.Model,
		bedrockProvider.WithBaseURL(replay.Server.URL),
		bedrockProvider.WithRegion("us-east-1"),
		bedrockProvider.WithCredentials(creds),
	)
	countedModel := &countingUnaryModel{LanguageModel: realModel}
	var resolverCalls atomic.Int32
	handler, err := providerwirev4.NewHandler(providerwirev4.ModelResolverFunc(func(_ *http.Request, modelID string) (provider.LanguageModel, error) {
		resolverCalls.Add(1)
		assert.Equal(t, cfg.Model, modelID)
		return countedModel, nil
	}))
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	requestBody := map[string]any{
		"prompt": []any{map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"type": "text", "text": cfg.Prompt}},
		}},
	}
	if cfg.ResponseFormat != nil {
		responseFormat := map[string]any{"type": cfg.ResponseFormat.Type, "schema": cfg.ResponseFormat.Schema}
		if cfg.ResponseFormat.Name != "" {
			responseFormat["name"] = cfg.ResponseFormat.Name
		}
		if cfg.ResponseFormat.Description != "" {
			responseFormat["description"] = cfg.ResponseFormat.Description
		}
		requestBody["responseFormat"] = responseFormat
	}
	encodedRequest, err := json.Marshal(requestBody)
	require.NoError(t, err)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+providerwirev4.PathLanguageModel, bytes.NewReader(encodedRequest))
	require.NoError(t, err)
	request.Header.Set("Content-Type", providerwirev4.MIMEJSON)
	request.Header.Set("Accept", "*/*")
	request.Header.Set(providerwirev4.HeaderModelID, cfg.Model)
	request.Header.Set(providerwirev4.HeaderSpecVersion, providerwirev4.SpecVersionV4)
	request.Header.Set(providerwirev4.HeaderStreaming, "false")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	assert.Equal(t, providerwirev4.MIMEJSON, response.Header.Get("Content-Type"))

	artifactBytes, err := os.ReadFile(filepath.Join("..", "..", "interop", "providerwire-v4", "generated", "bedrock-json-tool-with-answer.normalized.json"))
	require.NoError(t, err)
	var artifact normalizedUnaryArtifact
	require.NoError(t, json.Unmarshal(artifactBytes, &artifact))
	require.NotEmpty(t, artifact.Result)
	assert.JSONEq(t, string(artifact.Result), string(body))

	assert.Equal(t, int32(1), resolverCalls.Load())
	assert.Equal(t, int32(1), countedModel.generateCalls.Load())
	assert.Zero(t, countedModel.streamCalls.Load())
	assert.Equal(t, 1, replay.RequestCount())

	expectedRequests, err := conformance.LoadExpectedRequests(filepath.Join(fixtureDir, "expected-requests.jsonl"))
	require.NoError(t, err)
	conformance.CompareRequestSnapshots(t, expectedRequests, replay.Requests())
}
