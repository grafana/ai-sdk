package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The anthropic-sdk-go vertex helper panics on misconfiguration (empty
// region, missing default credentials, HTTP client setup failure). These
// tests verify that NewVertex and the underlying vertexAuth helper convert
// those panics into ordinary errors so the ai-sdk "never panic" rule holds
// at the API boundary.

func TestModel_InvalidToolDoesNotInvokeProvider(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	model := New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client())))
	options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.web_search_20250305", Name: "search", Args: map[string]json.RawMessage{}, Strict: boolPtr(false)}}}
	_, err := model.DoGenerate(context.Background(), options)
	require.Error(t, err)
	_, err = model.DoStream(context.Background(), options)
	require.Error(t, err)
	assert.Zero(t, requests)
}

func TestNewVertex_PanicRecovery(t *testing.T) {
	t.Run("empty location returns error", func(t *testing.T) {
		require.NotPanics(t, func() {
			model, err := NewVertex(context.Background(), "", "my-project", "claude-sonnet-4-5")
			assert.Nil(t, model)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "anthropic vertex auth")
			assert.Contains(t, err.Error(), "region must be provided")
		})
	})

	t.Run("missing default credentials returns error", func(t *testing.T) {
		// Point ADC at a path that does not exist so FindDefaultCredentials
		// fails deterministically regardless of host environment.
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/path/credentials.json")

		require.NotPanics(t, func() {
			model, err := NewVertex(context.Background(), "us-east5", "my-project", "claude-sonnet-4-5")
			assert.Nil(t, model)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "anthropic vertex auth")
		})
	})
}

func TestVertexAuth_PanicRecovery(t *testing.T) {
	t.Run("recovers string panic", func(t *testing.T) {
		require.NotPanics(t, func() {
			opt, err := vertexAuth(context.Background(), "", "my-project")
			assert.Nil(t, opt)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "anthropic vertex auth")
			assert.Contains(t, err.Error(), "region must be provided")
		})
	})

	t.Run("recovers error panic and preserves wrapped error", func(t *testing.T) {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/path/credentials.json")

		opt, err := vertexAuth(context.Background(), "us-east5", "my-project")
		assert.Nil(t, opt)
		require.Error(t, err)

		// The underlying SDK panics with a wrapped *fmt.wrapError carrying the
		// google.FindDefaultCredentials failure. We expect errors.Unwrap to
		// reach that inner error so callers can do typed checks.
		assert.Contains(t, err.Error(), "anthropic vertex auth")
		inner := errors.Unwrap(err)
		require.NotNil(t, inner, "expected wrapped inner error from panic recovery")
		assert.Contains(t, inner.Error(), "failed to find default credentials")
	})
}

func TestVertexAuth_RequestsCloudPlatformScope(t *testing.T) {
	originalGoogleAuth := vertexGoogleAuth
	t.Cleanup(func() { vertexGoogleAuth = originalGoogleAuth })

	var gotScopes []string
	vertexGoogleAuth = func(_ context.Context, _, _ string, scopes ...string) option.RequestOption {
		gotScopes = append([]string(nil), scopes...)
		return option.WithAPIKey("test-key")
	}

	opt, err := vertexAuth(context.Background(), "us-east5", "my-project")
	require.NoError(t, err)
	require.NotNil(t, opt)
	assert.Equal(t, []string{vertexCloudPlatformScope}, gotScopes)
}
