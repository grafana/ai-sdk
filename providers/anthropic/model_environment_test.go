package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_IgnoresEnvironmentDefaults(t *testing.T) {
	environmentNames := []string{
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_PROFILE",
		"ANTHROPIC_CONFIG_DIR",
		"ANTHROPIC_FEDERATION_RULE_ID",
		"ANTHROPIC_ORGANIZATION_ID",
		"ANTHROPIC_IDENTITY_TOKEN_FILE",
		"ANTHROPIC_IDENTITY_TOKEN",
		"ANTHROPIC_CUSTOM_HEADERS",
	}
	identityTokenPath := filepath.Join(t.TempDir(), "identity-token")
	require.NoError(t, os.WriteFile(identityTokenPath, []byte("poison-identity-token"), 0o600))
	explicitProfileDir := t.TempDir()
	writePoisonProfile(t, explicitProfileDir, "work", identityTokenPath)
	fallbackProfileDir := t.TempDir()
	writePoisonProfile(t, fallbackProfileDir, "default", identityTokenPath)
	require.NoError(t, os.WriteFile(filepath.Join(fallbackProfileDir, "active_config"), []byte("default"), 0o600))

	environmentCases := []struct {
		name string
		set  map[string]string
	}{
		{name: "base URL and credentials", set: map[string]string{
			"ANTHROPIC_API_KEY":    "environment-api-key",
			"ANTHROPIC_AUTH_TOKEN": "environment-auth-token",
		}},
		{name: "explicit profile", set: map[string]string{
			"ANTHROPIC_PROFILE":    "work",
			"ANTHROPIC_CONFIG_DIR": explicitProfileDir,
		}},
		{name: "federation", set: map[string]string{
			"ANTHROPIC_CONFIG_DIR":          t.TempDir(),
			"ANTHROPIC_FEDERATION_RULE_ID":  "poison-rule",
			"ANTHROPIC_ORGANIZATION_ID":     "poison-organization",
			"ANTHROPIC_IDENTITY_TOKEN_FILE": identityTokenPath,
			"ANTHROPIC_IDENTITY_TOKEN":      "poison-identity-token",
		}},
		{name: "fallback profile", set: map[string]string{
			"ANTHROPIC_CONFIG_DIR": fallbackProfileDir,
		}},
		{name: "custom headers", set: map[string]string{
			"ANTHROPIC_CONFIG_DIR":     t.TempDir(),
			"ANTHROPIC_CUSTOM_HEADERS": "X-Env-Leak: poisoned\nAuthorization: Bearer poisoned",
		}},
	}
	operations := []struct {
		name string
		run  func(t *testing.T, model provider.LanguageModel)
	}{
		{name: "unary", run: func(t *testing.T, model provider.LanguageModel) {
			result, err := model.DoGenerate(context.Background(), environmentTestCallOptions())
			require.NoError(t, err)
			require.NotNil(t, result)
		}},
		{name: "streaming", run: func(t *testing.T, model provider.LanguageModel) {
			result, err := model.DoStream(context.Background(), environmentTestCallOptions())
			require.NoError(t, err)
			for range result.Stream {
			}
		}},
	}

	for _, environmentCase := range environmentCases {
		t.Run(environmentCase.name, func(t *testing.T) {
			for _, operation := range operations {
				t.Run(operation.name, func(t *testing.T) {
					for _, name := range environmentNames {
						t.Setenv(name, "")
					}

					poison := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						t.Error("poison Anthropic endpoint received a request")
					}))
					defer poison.Close()
					t.Setenv("ANTHROPIC_BASE_URL", poison.URL)
					for name, value := range environmentCase.set {
						t.Setenv(name, value)
					}

					requests := make(chan *http.Request, 1)
					explicit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
						requests <- request.Clone(context.Background())
						if strings.Contains(request.Header.Get("Accept"), "text/event-stream") {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-test\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
							_, _ = fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n\n")
							_, _ = fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
							return
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"Hello"}],"model":"claude-test","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`)
					}))
					defer explicit.Close()

					model := New("explicit-api-key", "claude-test", WithRequestOptions(
						option.WithBaseURL(explicit.URL),
						option.WithHTTPClient(explicit.Client()),
						option.WithMaxRetries(0),
						option.WithHeader("X-Explicit", "present"),
					))
					operation.run(t, model)

					select {
					case request := <-requests:
						assert.Equal(t, "/v1/messages", request.URL.Path)
						assert.Equal(t, "explicit-api-key", request.Header.Get("X-Api-Key"))
						assert.Empty(t, request.Header.Values("Authorization"))
						assert.Empty(t, request.Header.Values("X-Env-Leak"))
						assert.Equal(t, "present", request.Header.Get("X-Explicit"))
					default:
						t.Fatal("explicit Anthropic endpoint received no request")
					}
				})
			}
		})
	}
}

func writePoisonProfile(t *testing.T, directory, name, identityTokenPath string) {
	t.Helper()
	configDirectory := filepath.Join(directory, "configs")
	require.NoError(t, os.MkdirAll(configDirectory, 0o700))
	profile := fmt.Sprintf(`{"authentication":{"type":"oidc_federation","federation_rule_id":"poison-rule","identity_token":{"source":"file","path":%q}},"organization_id":"poison-organization"}`, identityTokenPath)
	require.NoError(t, os.WriteFile(filepath.Join(configDirectory, name+".json"), []byte(profile), 0o600))
}

func environmentTestCallOptions() provider.CallOptions {
	maxOutputTokens := 64
	return provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hello")},
		MaxOutputTokens: &maxOutputTokens,
	}
}
