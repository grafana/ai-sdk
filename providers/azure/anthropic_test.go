package azure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/grafana/ai-sdk/providers/azure/foundry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const foundrySyntheticResponse = `{"id":"msg_foundry","type":"message","role":"assistant","model":"sweden-sonnet","content":[{"type":"thinking","thinking":"checking","signature":"signature"},{"type":"text","text":"ok"},{"type":"tool_use","id":"tool_1","name":"lookup","input":{"service":"api"}}],"stop_reason":"tool_use","usage":{"input_tokens":11,"output_tokens":3,"cache_read_input_tokens":7,"cache_creation_input_tokens":5}}`

const foundrySyntheticStream = `event: message_start
data: {"type":"message_start","message":{"id":"msg_foundry","type":"message","role":"assistant","model":"sweden-sonnet","content":[],"usage":{"input_tokens":11,"output_tokens":0,"cache_read_input_tokens":7,"cache_creation_input_tokens":5}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"checking"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"signature"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"ok"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: content_block_start
data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tool_1","name":"lookup","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"service\":\"api\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":2}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}

event: message_stop
data: {"type":"message_stop"}

`

func TestNewAnthropic_ProtocolAndIdentity(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/anthropic/v1/messages", r.URL.Path)
				assert.Empty(t, r.URL.Query().Get("api-version"))
				assert.Equal(t, "foundry-test-key", r.Header.Get("x-api-key"))
				assert.Empty(t, r.Header.Get("api-key"))
				assert.Empty(t, r.Header.Get("Authorization"))
				var body map[string]json.RawMessage
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.JSONEq(t, `"sweden-sonnet"`, string(body["model"]))
				assert.Contains(t, string(body["thinking"]), "adaptive")
				assert.Contains(t, string(body["output_config"]), "low")
				assert.Contains(t, string(body["tools"]), "lookup")
				assert.Contains(t, string(body["messages"]), "cache_control")
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, err := fmt.Fprint(w, foundrySyntheticStream)
					assert.NoError(t, err)
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, err := fmt.Fprint(w, foundrySyntheticResponse)
					assert.NoError(t, err)
				}
			}))
			t.Cleanup(server.Close)
			t.Setenv("ANTHROPIC_API_KEY", "ambient-key-must-not-leak")
			t.Setenv("ANTHROPIC_AUTH_TOKEN", "ambient-token-must-not-leak")
			t.Setenv("ANTHROPIC_BASE_URL", "https://ambient.invalid")
			t.Setenv("ANTHROPIC_FOUNDRY_RESOURCE", "ambient-resource-must-not-leak")
			t.Setenv("ANTHROPIC_FOUNDRY_BASE_URL", "https://ambient-foundry.invalid")
			t.Setenv("ANTHROPIC_FOUNDRY_API_KEY", "ambient-foundry-key-must-not-leak")
			model, err := NewAnthropic(foundry.Config{BaseURL: server.URL, APIKey: "foundry-test-key"}, "claude-sonnet-4-6", "sweden-sonnet")
			require.NoError(t, err)
			assert.Equal(t, "azure", model.Provider())
			assert.Equal(t, "claude-sonnet-4-6", model.ModelID())
			message := provider.UserText("hello")
			message.ProviderOptions = provider.BuildProviderOptions(anthropic.CacheControl("ephemeral"))
			tokens := 4096
			opts := provider.CallOptions{MaxOutputTokens: &tokens, Prompt: []provider.Message{message}, Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object","properties":{"service":{"type":"string"}}}`)}}}
			require.NoError(t, json.Unmarshal([]byte(`{"reasoning":"low"}`), &opts))
			var usage provider.Usage
			if stream {
				result, err := model.DoStream(t.Context(), opts)
				require.NoError(t, err)
				text, thinking, toolName, toolInput := "", "", "", ""
				metadataCount, finishCount := 0, 0
				for part := range result.Stream {
					require.NotEqual(t, provider.PartError, part.Type, "%v", part.APICallError)
					switch part.Type {
					case provider.PartResponseMeta:
						metadataCount++
						assert.Equal(t, "azure", part.Provider)
						assert.Equal(t, "claude-sonnet-4-6", part.ModelID)
					case provider.PartTextDelta:
						text += part.Delta
					case provider.PartReasoningDelta:
						thinking += part.Delta
					case provider.PartToolCall:
						toolName, toolInput = part.ToolName, part.Input
					case provider.PartFinish:
						finishCount++
						require.NotNil(t, part.Usage)
						usage = *part.Usage
					}
				}
				assert.Equal(t, 1, metadataCount)
				assert.Equal(t, 1, finishCount)
				assert.Equal(t, "ok", text)
				assert.Equal(t, "checking", thinking)
				assert.Equal(t, "lookup", toolName)
				assert.JSONEq(t, `{"service":"api"}`, toolInput)
			} else {
				result, err := model.DoGenerate(t.Context(), opts)
				require.NoError(t, err)
				require.NotNil(t, result.Response)
				assert.Equal(t, "azure", result.Response.Provider)
				assert.Equal(t, "claude-sonnet-4-6", result.Response.ModelID)
				require.Len(t, result.Content, 3)
				assert.Equal(t, "checking", result.Content[0].Text)
				assert.Equal(t, "ok", result.Content[1].Text)
				assert.Equal(t, "lookup", result.Content[2].ToolName)
				assert.JSONEq(t, `{"service":"api"}`, string(result.Content[2].Input))
				usage = result.Usage
			}
			assert.Equal(t, 23, *usage.InputTokens.Total)
			assert.Equal(t, 11, *usage.InputTokens.NoCache)
			assert.Equal(t, 7, *usage.InputTokens.CacheRead)
			assert.Equal(t, 5, *usage.InputTokens.CacheWrite)
			assert.Equal(t, 3, *usage.OutputTokens.Total)
		})
	}
}

func TestNewAnthropic_Credentials(t *testing.T) {
	for _, tc := range []struct {
		name        string
		credentials []foundry.Credential
		retry       bool
	}{
		{name: "rotates key between requests", credentials: []foundry.Credential{{APIKey: "first"}, {APIKey: "second"}}},
		{name: "rotates bearer between requests", credentials: []foundry.Credential{{AuthToken: "first"}, {AuthToken: "second"}}},
		{name: "refreshes on retry", credentials: []foundry.Credential{{APIKey: "first"}, {APIKey: "second"}}, retry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				i := int(attempts.Add(1)) - 1
				if !assert.Less(t, i, len(tc.credentials)) {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var body map[string]json.RawMessage
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.JSONEq(t, `"sweden-sonnet"`, string(body["model"]))
				want := tc.credentials[i]
				assert.Equal(t, want.APIKey, r.Header.Get("x-api-key"))
				assert.Empty(t, r.Header.Get("api-key"))
				if want.AuthToken != "" {
					assert.Equal(t, "Bearer "+want.AuthToken, r.Header.Get("Authorization"))
				} else {
					assert.Empty(t, r.Header.Get("Authorization"))
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.retry && i == 0 {
					w.Header().Set("retry-after-ms", "1")
					w.WriteHeader(http.StatusTooManyRequests)
					_, err := fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"retry"}}`)
					assert.NoError(t, err)
					return
				}
				_, err := fmt.Fprint(w, foundrySyntheticResponse)
				assert.NoError(t, err)
			}))
			t.Cleanup(server.Close)
			var refreshes atomic.Int32
			config := foundry.Config{BaseURL: server.URL, Credential: func(ctx context.Context) (foundry.Credential, error) {
				if err := ctx.Err(); err != nil {
					return foundry.Credential{}, err
				}
				i := int(refreshes.Add(1)) - 1
				if i >= len(tc.credentials) {
					return foundry.Credential{}, errors.New("unexpected credential refresh")
				}
				return tc.credentials[i], nil
			}}
			model, err := NewAnthropic(config, "claude-sonnet-4-6", "sweden-sonnet", anthropic.WithRequestOptions(option.WithMaxRetries(1)))
			require.NoError(t, err)
			requests := 2
			if tc.retry {
				requests = 1
			}
			for range requests {
				_, err = model.DoGenerate(t.Context(), foundryCallOptions())
				require.NoError(t, err)
			}
			assert.EqualValues(t, 2, attempts.Load())
			assert.EqualValues(t, 2, refreshes.Load())
		})
	}
}

func TestNewAnthropic_CredentialFailures(t *testing.T) {
	failure := errors.New("credential service unavailable")
	for _, tc := range []struct {
		name       string
		credential foundry.Credential
		err        error
	}{
		{name: "empty"},
		{name: "ambiguous", credential: foundry.Credential{APIKey: "key", AuthToken: "token"}},
		{name: "callback error", err: failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(server.Close)
			model, err := NewAnthropic(foundry.Config{BaseURL: server.URL, Credential: func(context.Context) (foundry.Credential, error) { return tc.credential, tc.err }}, "claude-sonnet-4-6", "sweden-sonnet", anthropic.WithRequestOptions(option.WithMaxRetries(0)))
			require.NoError(t, err)
			_, err = model.DoGenerate(t.Context(), foundryCallOptions())
			require.Error(t, err)
			assert.Zero(t, attempts.Load())
		})
	}
}

func TestNewAnthropic_RejectsRedirect(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, foundrySyntheticResponse)
	}))
	t.Cleanup(destination.Close)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(origin.Close)
	model, err := NewAnthropic(foundry.Config{BaseURL: origin.URL, APIKey: "must-not-follow"}, "claude-sonnet-4-6", "sweden-sonnet", anthropic.WithRequestOptions(option.WithMaxRetries(0)))
	require.NoError(t, err)
	_, err = model.DoGenerate(t.Context(), foundryCallOptions())
	require.Error(t, err)
	assert.Zero(t, destinationCalls.Load())
}

func TestNewAnthropic_Cancellation(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			started := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				close(started)
				<-r.Context().Done()
			}))
			t.Cleanup(server.Close)
			model, err := NewAnthropic(foundry.Config{BaseURL: server.URL, APIKey: "key"}, "claude-sonnet-4-6", "sweden-sonnet", anthropic.WithRequestOptions(option.WithMaxRetries(0)))
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				opts := foundryCallOptions()
				if stream {
					_, err := model.DoStream(ctx, opts)
					done <- err
				} else {
					_, err := model.DoGenerate(ctx, opts)
					done <- err
				}
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not start")
			}
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				t.Fatal("cancellation did not finish request")
			}
		})
	}
}

func foundryCallOptions() provider.CallOptions {
	tokens := 256
	return provider.CallOptions{MaxOutputTokens: &tokens, Prompt: []provider.Message{provider.UserText("hello")}}
}

func TestNewAnthropic_InvalidIdentity(t *testing.T) {
	for _, tc := range []struct{ name, modelID, deployment string }{
		{name: "missing canonical model", deployment: "deployment"},
		{name: "missing deployment", modelID: "claude-sonnet-4-6"},
		{name: "canonical whitespace", modelID: " claude-sonnet-4-6", deployment: "deployment"},
		{name: "deployment whitespace", modelID: "claude-sonnet-4-6", deployment: "deployment "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, err := NewAnthropic(foundry.Config{BaseURL: "https://example.services.ai.azure.com", APIKey: "key"}, tc.modelID, tc.deployment)
			require.Error(t, err)
			assert.Nil(t, model)
		})
	}
}

func TestNewAnthropic_StaticBearerOverridesOtherAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer foundry-token", r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("x-api-key"))
		assert.Empty(t, r.Header.Get("api-key"))
		w.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprint(w, foundrySyntheticResponse)
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	model, err := NewAnthropic(foundry.Config{BaseURL: server.URL, AuthToken: "foundry-token"}, "claude-sonnet-4-6", "sweden-sonnet", anthropic.WithRequestOptions(option.WithAPIKey("wrong-key"), option.WithHeader("api-key", "wrong-azure-key"), option.WithAuthToken("wrong-token")))
	require.NoError(t, err)
	_, err = model.DoGenerate(t.Context(), foundryCallOptions())
	require.NoError(t, err)
}

func TestNewAnthropic_HTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		retryable bool
	}{
		{name: "authentication", status: http.StatusUnauthorized},
		{name: "unavailable deployment", status: http.StatusNotFound},
		{name: "throttled", status: http.StatusTooManyRequests, retryable: true},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				var attempts atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempts.Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					_, err := fmt.Fprint(w, `{"type":"error","error":{"type":"api_error","message":"test failure"}}`)
					assert.NoError(t, err)
				}))
				t.Cleanup(server.Close)
				model, err := NewAnthropic(foundry.Config{BaseURL: server.URL, APIKey: "key"}, "claude-sonnet-4-6", "sweden-sonnet", anthropic.WithRequestOptions(option.WithMaxRetries(0)))
				require.NoError(t, err)
				if stream {
					_, err = model.DoStream(t.Context(), foundryCallOptions())
				} else {
					_, err = model.DoGenerate(t.Context(), foundryCallOptions())
				}
				var apiErr *provider.APICallError
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, tc.status, apiErr.StatusCode)
				assert.Equal(t, tc.retryable, apiErr.IsRetryable)
				assert.EqualValues(t, 1, attempts.Load())
			})
		}
	}
}

func TestNewAnthropic_IgnoresAmbientCredentialChains(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*testing.T)
	}{
		{name: "broken profile", configure: func(t *testing.T) { t.Setenv("ANTHROPIC_PROFILE", "nonexistent-profile") }},
		{name: "federation", configure: func(t *testing.T) {
			t.Setenv("ANTHROPIC_FEDERATION_RULE_ID", "ambient-rule")
			t.Setenv("ANTHROPIC_ORGANIZATION_ID", "ambient-organization")
			t.Setenv("ANTHROPIC_IDENTITY_TOKEN", "ambient-identity-must-not-be-exchanged")
		}},
		{name: "custom headers", configure: func(t *testing.T) {
			t.Setenv("ANTHROPIC_CUSTOM_HEADERS", "X-Ambient-Secret: ambient-secret-must-not-leak\nAuthorization: Bearer ambient-token-must-not-leak")
		}},
	} {
		for _, callback := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/callback=%t", tc.name, callback), func(t *testing.T) {
				for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_PROFILE", "ANTHROPIC_FEDERATION_RULE_ID", "ANTHROPIC_ORGANIZATION_ID", "ANTHROPIC_IDENTITY_TOKEN_FILE", "ANTHROPIC_IDENTITY_TOKEN", "ANTHROPIC_CUSTOM_HEADERS"} {
					t.Setenv(key, "")
					require.NoError(t, os.Unsetenv(key))
				}
				t.Setenv("ANTHROPIC_CONFIG_DIR", t.TempDir())
				tc.configure(t)
				var messages, otherCalls atomic.Int32
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/anthropic/v1/messages" {
						otherCalls.Add(1)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					messages.Add(1)
					assert.Equal(t, "explicit-foundry-key", r.Header.Get("x-api-key"))
					assert.Empty(t, r.Header.Get("Authorization"))
					assert.Empty(t, r.Header.Get("X-Ambient-Secret"))
					w.Header().Set("Content-Type", "application/json")
					_, err := fmt.Fprint(w, foundrySyntheticResponse)
					assert.NoError(t, err)
				}))
				t.Cleanup(server.Close)
				config := foundry.Config{BaseURL: server.URL, APIKey: "explicit-foundry-key"}
				if callback {
					config.APIKey = ""
					config.Credential = func(context.Context) (foundry.Credential, error) {
						return foundry.Credential{APIKey: "explicit-foundry-key"}, nil
					}
				}
				model, err := NewAnthropic(config, "claude-sonnet-4-6", "sweden-sonnet", anthropic.WithRequestOptions(option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
				require.NoError(t, err)
				_, err = model.DoGenerate(t.Context(), foundryCallOptions())
				assert.NoError(t, err)
				assert.EqualValues(t, 1, messages.Load())
				assert.Zero(t, otherCalls.Load(), "ambient credentials must not trigger token exchange")
			})
		}
	}
}
