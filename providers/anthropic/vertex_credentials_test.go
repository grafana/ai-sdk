package anthropic

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	vertexsdk "github.com/anthropics/anthropic-sdk-go/vertex"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const fakeTokenURL = "https://oauth2.vertex.test/token"

type fakeVertexRequest struct {
	url           string
	authorization string
	apiKey        string
	body          string
}

// fakeVertex answers the token endpoint and Vertex rawPredict through one
// transport, so tests see every request the configured client sends.
type fakeVertex struct {
	mu       sync.Mutex
	tokens   []string
	requests []fakeVertexRequest
}

func (f *fakeVertex) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(request.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	if request.URL.String() == fakeTokenURL {
		f.tokens = append(f.tokens, string(body))
		return jsonResponse(request, `{"access_token":"vertex-token","token_type":"Bearer","expires_in":3600}`), nil
	}
	f.requests = append(f.requests, fakeVertexRequest{url: request.URL.String(), authorization: request.Header.Get("Authorization"), apiKey: request.Header.Get("X-Api-Key"), body: string(body)})
	return jsonResponse(request, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"hello from vertex"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2}}`), nil
}

func jsonResponse(request *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}
}

func serviceAccountJSON(t *testing.T, credentialType string) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: must(x509.MarshalPKCS8PrivateKey(key))})
	raw, err := json.Marshal(map[string]string{
		"type":           credentialType,
		"project_id":     "key-project",
		"private_key_id": "key-1",
		"private_key":    string(encoded),
		"client_email":   "gateway@key-project.iam.gserviceaccount.com",
		"token_uri":      fakeTokenURL,
	})
	require.NoError(t, err)
	return raw
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func generateHello(t *testing.T, m provider.LanguageModel) *provider.GenerateResult {
	t.Helper()
	result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: new(16)})
	require.NoError(t, err)
	return result
}

func TestNewVertexWithCredentials(t *testing.T) {
	t.Run("service account authorizes the configured client and ignores the environment", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "hostile-anthropic-key")
		t.Setenv("ANTHROPIC_BASE_URL", "https://hostile.example.test")
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/hostile.json")
		fake := &fakeVertex{}
		m, err := NewVertexWithCredentials(context.Background(), "us-east5", "configured-project", "claude-sonnet-4-5", serviceAccountJSON(t, "service_account"),
			WithHTTPClient(&http.Client{Transport: fake}), WithRequestOptions(option.WithMaxRetries(0)))
		require.NoError(t, err)
		result := generateHello(t, m)
		require.Len(t, result.Content, 1)
		assert.Equal(t, "hello from vertex", result.Content[0].Text)
		require.Len(t, fake.tokens, 1)
		assert.Contains(t, fake.tokens[0], "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer")
		require.Len(t, fake.requests, 1)
		assert.Equal(t, "https://us-east5-aiplatform.googleapis.com/v1/projects/configured-project/locations/us-east5/publishers/anthropic/models/claude-sonnet-4-5@20250929:rawPredict", strings.Split(fake.requests[0].url, "?")[0])
		assert.Equal(t, "Bearer vertex-token", fake.requests[0].authorization)
		assert.Empty(t, fake.requests[0].apiKey)
	})

	t.Run("rejects other credential types", func(t *testing.T) {
		_, err := NewVertexWithCredentials(context.Background(), "us-east5", "configured-project", "claude-sonnet-4-5", serviceAccountJSON(t, "authorized_user"))
		require.ErrorContains(t, err, "anthropic vertex credentials")
	})

	t.Run("rejects malformed credentials without echoing them", func(t *testing.T) {
		_, err := NewVertexWithCredentials(context.Background(), "us-east5", "configured-project", "claude-sonnet-4-5", []byte(`{"type":"service_account","private_key":"not-a-key-secret-material"}`))
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "secret-material")
	})

	t.Run("requires location and project", func(t *testing.T) {
		for _, tc := range []struct{ location, project string }{{"", "configured-project"}, {"us-east5", ""}} {
			_, err := NewVertexWithCredentials(context.Background(), tc.location, tc.project, "claude-sonnet-4-5", serviceAccountJSON(t, "service_account"))
			require.ErrorContains(t, err, "location and project ID are required")
		}
	})
}

func TestNewVertex_WithHTTPClientKeepsAuthorization(t *testing.T) {
	original := vertexGoogleAuth
	t.Cleanup(func() { vertexGoogleAuth = original })
	creds := &google.Credentials{ProjectID: "configured-project", TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "adc-token"})}
	vertexGoogleAuth = func(ctx context.Context, region, project string, _ ...string) option.RequestOption {
		return vertexsdk.WithCredentials(ctx, region, project, creds)
	}
	fake := &fakeVertex{}
	m, err := NewVertex(context.Background(), "us-east5", "configured-project", "claude-sonnet-4-5", WithHTTPClient(&http.Client{Transport: fake}), WithRequestOptions(option.WithMaxRetries(0)))
	require.NoError(t, err)
	generateHello(t, m)
	require.Len(t, fake.requests, 1)
	assert.Equal(t, "Bearer adc-token", fake.requests[0].authorization)
}

func TestNew_WithHTTPClient(t *testing.T) {
	fake := &fakeVertex{}
	m := New("direct-key", "claude-sonnet-4-5", WithHTTPClient(&http.Client{Transport: fake}), WithRequestOptions(option.WithMaxRetries(0)))
	generateHello(t, m)
	require.Len(t, fake.requests, 1)
	assert.Equal(t, "direct-key", fake.requests[0].apiKey)
	assert.True(t, strings.HasPrefix(fake.requests[0].url, "https://api.anthropic.com/v1/messages"), fake.requests[0].url)
}
