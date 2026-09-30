package v4

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderDiagnostics_DirectRoute(t *testing.T) {
	for _, tc := range []struct {
		name     string
		format   catalog.ProviderErrorFormat
		raw      string
		status   int
		typeName string
		param    string
	}{
		{name: "openai request", format: catalog.ProviderErrorOpenAI, status: 422, raw: `{"message":"fix this input","type":"invalid_request_error","code":42,"param":"temperature","unknown":"excluded"}`, typeName: "invalid_request_error", param: `{"type":"invalid_request_error","code":42,"param":"temperature"}`},
		{name: "anthropic conflict", format: catalog.ProviderErrorAnthropic, status: 409, raw: `{"type":"error","error":{"type":"api_error","message":"fix this input"},"request_id":"excluded"}`, typeName: "failed_dependency", param: `{"type":"api_error"}`},
		{name: "compatible limit", format: catalog.ProviderErrorOpenAI, status: 429, raw: `{"error":{"message":"fix this input","type":"rate_limit_error","code":null,"param":null}}`, typeName: "rate_limit_exceeded", param: `{"type":"rate_limit_error","code":null,"param":null}`},
		{name: "openai server", format: catalog.ProviderErrorOpenAI, status: 529, raw: `{"message":"fix this input","type":"server_error","code":"overloaded"}`, typeName: "internal_server_error", param: `{"type":"server_error","code":"overloaded"}`},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/unary", true: "/stream"}[streaming], func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				harness.resolver.resolved.ProviderErrors = tc.format
				original := provider.NewAPICallError(provider.APICallErrorOptions{Message: "excluded sdk dump", StatusCode: tc.status, Data: json.RawMessage(tc.raw), URL: "https://excluded.invalid", ResponseHeaders: map[string][]string{"Authorization": {"excluded"}}})
				wrapped := errors.Join(original, errors.New("excluded cause"))
				harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, original }
				harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartError, APICallError: original}, finishPart())}, nil
				}
				request := validRequest(`{"prompt":[]}`)
				if streaming {
					request = streamRequest(`{"prompt":[]}`)
				}
				response := harness.serve(request)
				var payload []byte
				if streaming {
					require.Equal(t, http.StatusOK, response.Code)
					requireStreamBodyMatchesSchema(t, response.Body.String())
					frames := strings.Split(strings.TrimSpace(response.Body.String()), "\n\n")
					payload = []byte(strings.TrimPrefix(frames[1], "data: "))
					assert.Contains(t, response.Body.String(), `"type":"finish"`)
				} else {
					require.Equal(t, tc.status, response.Code, response.Body.String())
					compiled, err := schema.CompileSchema(errorSchemaJSON)
					require.NoError(t, err)
					require.NoError(t, compiled.Validate(response.Body.Bytes()))
					payload = response.Body.Bytes()
				}
				var value struct {
					Error struct {
						Message    string
						Type       string
						Param      json.RawMessage
						StatusCode int
						Retryable  bool
					}
				}
				require.NoError(t, json.Unmarshal(payload, &value))
				assert.Equal(t, "fix this input", value.Error.Message)
				assert.Equal(t, tc.typeName, value.Error.Type)
				assert.JSONEq(t, tc.param, string(value.Error.Param))
				assert.NotContains(t, response.Body.String(), "excluded")
				assert.Equal(t, "excluded sdk dump", original.Message)
				var api *provider.APICallError
				require.ErrorAs(t, wrapped, &api)
				assert.Same(t, original, api)
			})
		}
	}
}

func TestProviderDiagnostics_ProjectionBoundsAndOwnership(t *testing.T) {
	original := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 409, ResponseBody: `{"error":{"message":"native diagnostic","type":"api_error","code":null,"param":"input"}}`, Message: "excluded dump"})
	before, err := json.Marshal(original)
	require.NoError(t, err)
	projection, ok := projectProviderError(original, catalog.ProviderErrorOpenAI, false)
	require.True(t, ok)
	assert.Equal(t, 409, projection.status)
	assert.JSONEq(t, `{"type":"api_error","code":null,"param":"input"}`, string(projection.Param))
	_, ok = projectProviderError(errors.Join(original, errors.New("different source")), catalog.ProviderErrorOpenAI, false)
	assert.False(t, ok)
	after, err := json.Marshal(original)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	for _, raw := range []string{
		`{"message":"native","type":"` + strings.Repeat("x", 257) + `"}`,
		`{"message":"native","code":"` + strings.Repeat("x", 257) + `"}`,
		`{"message":"native","code":true}`,
		`{"message":"native","param":{"unknown":"excluded"}}`,
		`{"message":"native","param":"` + strings.Repeat("x", 4097) + `"}`,
		string([]byte{'{', '"', 'm', 'e', 's', 's', 'a', 'g', 'e', '"', ':', '"', 255, '"', '}'}),
	} {
		_, ok = projectProviderError(provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 400, Data: json.RawMessage(raw)}), catalog.ProviderErrorOpenAI, false)
		assert.False(t, ok)
	}
	for _, streaming := range []bool{false, true} {
		limits := testLimits()
		limits.UnaryResponseBytes, limits.StreamFrameBytes = 512, 512
		harness := newRuntimeHarness(t, limits)
		harness.resolver.resolved.ProviderErrors = catalog.ProviderErrorOpenAI
		api := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 400, Data: json.RawMessage(`{"message":"` + strings.Repeat("<", 100) + `","type":"native"}`)})
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, api }
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartError, APICallError: api}, finishPart())}, nil
		}
		request := validRequest(`{"prompt":[]}`)
		if streaming {
			request = streamRequest(`{"prompt":[]}`)
		}
		response := harness.serve(request)
		assert.NotContains(t, response.Body.String(), `\\u003c`)
		assert.Contains(t, response.Body.String(), "failed dependency")
		if streaming {
			assert.Contains(t, response.Body.String(), `"type":"finish"`)
		}
	}
}

func TestProviderDiagnostics_StreamOwnershipAndWriterFailure(t *testing.T) {
	t.Run("result plus error remains precommit", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.resolver.resolved.ProviderErrors = catalog.ProviderErrorOpenAI
		api := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 409, Data: json.RawMessage(`{"message":"native conflict","code":null}`)})
		parts := make(chan provider.StreamPart, 1)
		parts <- provider.StreamPart{Type: provider.PartRaw}
		close(parts)
		var modelContext context.Context
		harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			modelContext = ctx
			return &provider.StreamResult{Stream: parts}, api
		}
		response := harness.serve(streamRequest(`{"prompt":[]}`))
		assert.Equal(t, 409, response.Code)
		assert.Contains(t, response.Body.String(), "native conflict")
		assert.NotContains(t, response.Body.String(), "data: ")
		require.ErrorIs(t, modelContext.Err(), context.Canceled)
		require.Eventually(t, func() bool { return len(parts) == 0 }, time.Second, time.Millisecond)
		generate, streaming := harness.model.invocationCounts()
		assert.Zero(t, generate)
		assert.Equal(t, 1, streaming)
		assert.Equal(t, 409, api.StatusCode)
	})
	for _, failure := range []string{"write", "flush"} {
		t.Run(failure, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.resolver.resolved.ProviderErrors = catalog.ProviderErrorOpenAI
			var modelContext context.Context
			api := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 500, Data: json.RawMessage(`{"message":"native failure","type":"server_error"}`)})
			harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				modelContext = ctx
				return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartError, APICallError: api}, finishPart())}, nil
			}
			writer := &responseWriterProbe{}
			if failure == "write" {
				writer.onWrite = func() {
					if writer.writes == 2 {
						writer.writeErr = errors.New("writer failed")
					}
				}
			} else {
				writer.flushErrors = []error{nil, nil, errors.New("flush failed")}
			}
			harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`))
			require.ErrorIs(t, modelContext.Err(), context.Canceled)
			assert.Equal(t, 2, writer.writes)
			assert.NotContains(t, writer.body.String(), `"type":"finish"`)
			_, streaming := harness.model.invocationCounts()
			assert.Equal(t, 1, streaming)
		})
	}
}

func TestProviderDiagnostics_SafeSources(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format catalog.ProviderErrorFormat
		raw    string
		status int
	}{
		{name: "unconfigured", raw: `{"message":"excluded","type":"api_error"}`, status: 400},
		{name: "unknown format", format: "unknown", raw: `{"message":"excluded"}`, status: 400},
		{name: "invalid json", format: catalog.ProviderErrorOpenAI, raw: `{`, status: 400},
		{name: "oversize", format: catalog.ProviderErrorOpenAI, raw: strings.Repeat(" ", 16385), status: 400},
		{name: "oversize message", format: catalog.ProviderErrorOpenAI, raw: `{"message":"` + strings.Repeat("x", 4097) + `"}`, status: 400},
		{name: "nonfinite code", format: catalog.ProviderErrorOpenAI, raw: `{"message":"excluded","code":1e400}`, status: 400},
		{name: "auth", format: catalog.ProviderErrorOpenAI, raw: `{"message":"excluded key","type":"authentication_error","param":"excluded key","code":"excluded key"}`, status: 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.resolver.resolved.ProviderErrors = tc.format
			harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: tc.status, Message: "excluded dump", Data: json.RawMessage(tc.raw)})
			}
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.NotContains(t, response.Body.String(), "excluded")
			if tc.name == "auth" {
				assert.Equal(t, http.StatusUnauthorized, response.Code)
				assert.Contains(t, response.Body.String(), "provider account authorization failed")
			}
		})
	}
}
