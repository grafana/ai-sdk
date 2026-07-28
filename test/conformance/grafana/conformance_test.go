//go:build conformance

package grafana_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/gateway/providerwire"
	"github.com/grafana/ai-sdk/provider"
	anthropicProvider "github.com/grafana/ai-sdk/providers/anthropic"
	grafanaProvider "github.com/grafana/ai-sdk/providers/grafana"
	"github.com/grafana/ai-sdk/test/conformance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const conformanceAccessToken = "conformance-access-token"

func TestConformance(t *testing.T) {
	providerDir := "../anthropic"
	cases := conformance.DiscoverTestCases(t, providerDir)

	if len(cases) == 0 {
		t.Skip("no conformance test cases found")
	}

	factory := func(baseURL string, cfg *conformance.Config) (provider.LanguageModel, error) {
		p, err := grafanaProvider.NewWithAccessToken(grafanaProvider.AccessTokenConfig{
			AccessToken: conformanceAccessToken,
			BaseURL:     baseURL,
		})
		if err != nil {
			return nil, err
		}
		return p.LanguageModel(cfg.Model)
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			conformance.RunTestCaseWithServer(t, tc, factory, newGrafanaProviderWireServer)
		})
	}
}

type grafanaProviderWireServer struct {
	server          *httptest.Server
	anthropicReplay *conformance.ReplayServer
	handler         http.Handler
	count           atomic.Int32
}

func newGrafanaProviderWireServer(t *testing.T, tc conformance.TestCase) (*conformance.TestServer, error) {
	t.Helper()
	anthropicReplay, err := conformance.NewReplayServer(tc.Dir, tc.Provider)
	if err != nil {
		return nil, err
	}

	gs := &grafanaProviderWireServer{anthropicReplay: anthropicReplay}
	gs.handler, err = providerwire.NewHandler(providerwire.ModelResolverFunc(func(r *http.Request, modelID string) (provider.LanguageModel, error) {
		return anthropicProvider.New(
			"test-api-key",
			modelID,
			anthropicProvider.WithRequestOptions(option.WithBaseURL(gs.anthropicReplay.Server.URL)),
		), nil
	}))
	if err != nil {
		anthropicReplay.Close()
		return nil, err
	}
	gs.server = httptest.NewServer(http.HandlerFunc(gs.handle))

	return &conformance.TestServer{
		BaseURL:      gs.server.URL,
		RequestCount: func() int { return int(gs.count.Load()) },
		Requests:     anthropicReplay.Requests,
		Close: func() {
			gs.server.Close()
			anthropicReplay.Close()
		},
	}, nil
}

func (gs *grafanaProviderWireServer) handle(w http.ResponseWriter, r *http.Request) {
	gs.count.Add(1)
	if err := validateProviderWireHostRequest(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	gs.handler.ServeHTTP(w, r)
}

func validateProviderWireHostRequest(r *http.Request) error {
	if r.URL.Path != providerwire.PathLanguageModel {
		return fmt.Errorf("invalid path")
	}
	if r.Header.Get("X-Access-Token") != conformanceAccessToken {
		return fmt.Errorf("invalid access token")
	}
	return nil
}

func TestUserIDTokenForwarding(t *testing.T) {
	var gotUserID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-Grafana-Id")
		assert.NoError(t, validateProviderWireHostRequest(r))
		_, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", providerwire.MIMESSE)
		w.WriteHeader(http.StatusOK)
		finish := provider.FinishReason{Unified: provider.FinishReasonStop}
		require.NoError(t, providerwire.WriteSSEStreamPartTo(w, provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish}))
	}))
	t.Cleanup(server.Close)

	p, err := grafanaProvider.NewWithAccessToken(grafanaProvider.AccessTokenConfig{
		AccessToken: conformanceAccessToken,
		BaseURL:     server.URL,
	})
	require.NoError(t, err)
	model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
	require.NoError(t, err)

	ctx := grafanaProvider.WithUserIDToken(context.Background(), "user-id-token")
	stream, err := model.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.NoError(t, err)
	for range stream.Stream {
	}
	assert.Equal(t, "user-id-token", gotUserID)
}

func TestProviderWireRequestBodyDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, validateProviderWireHostRequest(r))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &raw))
		assert.Contains(t, raw, "prompt")
		_, err = providerwire.DecodeCallOptions(body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", providerwire.MIMESSE)
		w.WriteHeader(http.StatusOK)
		finish := provider.FinishReason{Unified: provider.FinishReasonStop}
		require.NoError(t, providerwire.WriteSSEStreamPartTo(w, provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish}))
	}))
	t.Cleanup(server.Close)

	p, err := grafanaProvider.NewWithAccessToken(grafanaProvider.AccessTokenConfig{
		AccessToken: conformanceAccessToken,
		BaseURL:     server.URL,
	})
	require.NoError(t, err)
	model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
	require.NoError(t, err)

	stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.NoError(t, err)
	for range stream.Stream {
	}
}
