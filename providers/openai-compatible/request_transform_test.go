package openaicompatible

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestTransformAndWarnings_HTTP(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		name := "generate"
		if streaming {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			requests := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				requests <- body
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`)
				}
			}))
			defer server.Close()

			var incoming []provider.Warning
			transformCalls := 0
			added := provider.Warning{Type: provider.WarnCompatibility, Feature: "model", Details: "using a backend alias"}
			model := New("test-model", WithBaseURL(server.URL), WithIncludeUsage(true),
				WithRequestTransformAndWarnings(func(body map[string]any, warnings []provider.Warning) (map[string]any, []provider.Warning, error) {
					transformCalls++
					incoming = append([]provider.Warning(nil), warnings...)
					assert.Equal(t, "test-model", body["model"])
					assert.Contains(t, body, "messages")
					if streaming {
						assert.Equal(t, true, body["stream"])
						assert.Equal(t, streamOptions{IncludeUsage: true}, body["stream_options"])
					} else {
						assert.NotContains(t, body, "stream")
					}
					return map[string]any{"model": "backend-alias", "stream": streaming}, append(warnings, added), nil
				}))
			topK := 5
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, TopK: &topK}
			var warnings []provider.Warning
			var metadata *provider.RequestMetadata
			if streaming {
				result, err := model.DoStream(context.Background(), opts)
				require.NoError(t, err)
				metadata = result.Request
				var parts []provider.StreamPart
				for part := range result.Stream {
					parts = append(parts, part)
				}
				require.NotEmpty(t, parts)
				assert.Equal(t, provider.PartStreamStart, parts[0].Type)
				warnings = parts[0].Warnings
			} else {
				result, err := model.DoGenerate(context.Background(), opts)
				require.NoError(t, err)
				metadata, warnings = result.Request, result.Warnings
			}
			assert.Equal(t, 1, transformCalls)
			require.Len(t, incoming, 1)
			assert.Equal(t, "topK", incoming[0].Feature)
			assert.Equal(t, append(incoming, added), warnings)
			require.NotNil(t, metadata)
			body := <-requests
			assert.Equal(t, json.RawMessage(body), metadata.Body)
			assert.JSONEq(t, `{"model":"backend-alias","stream":`+strconv.FormatBool(streaming)+`}`, string(body))
		})
	}
}

func TestRequestTransform_Options(t *testing.T) {
	added := provider.Warning{Type: provider.WarnOther, Message: "transformed"}
	legacy := WithRequestTransform(func(body map[string]any) (map[string]any, error) {
		body["legacy"] = true
		return body, nil
	})
	withWarnings := WithRequestTransformAndWarnings(func(body map[string]any, warnings []provider.Warning) (map[string]any, []provider.Warning, error) {
		body["warningAware"] = true
		return body, append(warnings, added), nil
	})
	for _, tc := range []struct {
		name string
		opts []Option
		want string
	}{
		{name: "legacy", opts: []Option{legacy}, want: "legacy"},
		{name: "warning-aware", opts: []Option{withWarnings}, want: "warningAware"},
		{name: "legacy replaces warning-aware", opts: []Option{withWarnings, legacy}, want: "legacy"},
		{name: "warning-aware replaces legacy", opts: []Option{legacy, withWarnings}, want: "warningAware"},
		{name: "nil legacy disables warning-aware", opts: []Option{withWarnings, WithRequestTransform(nil)}},
		{name: "nil warning-aware disables legacy", opts: []Option{legacy, WithRequestTransformAndWarnings(nil)}},
		{name: "no transform"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &model{modelID: "test-model"}
			for _, opt := range tc.opts {
				opt(m)
			}
			topK := 5
			body, warnings, err := m.buildRequest(provider.CallOptions{TopK: &topK}, false)
			require.NoError(t, err)
			assert.Equal(t, "test-model", body["model"])
			if tc.want != "" {
				assert.Equal(t, true, body[tc.want])
			}
			for _, key := range []string{"legacy", "warningAware"} {
				if key != tc.want {
					assert.NotContains(t, body, key)
				}
			}
			require.NotEmpty(t, warnings)
			assert.Equal(t, "topK", warnings[0].Feature)
			if tc.want == "warningAware" {
				require.Len(t, warnings, 2)
				assert.Equal(t, added, warnings[1])
			} else {
				assert.Len(t, warnings, 1)
			}
		})
	}
}

func TestRequestTransform_RejectsBeforeHTTP(t *testing.T) {
	transformErr := errors.New("cannot adapt body")
	for _, tc := range []struct {
		name string
		opt  Option
		err  error
	}{
		{name: "legacy error", err: transformErr, opt: WithRequestTransform(func(body map[string]any) (map[string]any, error) {
			return body, transformErr
		})},
		{name: "warning-aware error", err: transformErr, opt: WithRequestTransformAndWarnings(func(body map[string]any, warnings []provider.Warning) (map[string]any, []provider.Warning, error) {
			return body, append(warnings, provider.Warning{Type: provider.WarnOther}), transformErr
		})},
		{name: "legacy nil body", opt: WithRequestTransform(func(map[string]any) (map[string]any, error) {
			return nil, nil
		})},
		{name: "warning-aware nil body", opt: WithRequestTransformAndWarnings(func(map[string]any, []provider.Warning) (map[string]any, []provider.Warning, error) {
			return nil, nil, nil
		})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return nil, errors.New("unexpected HTTP request")
			})}
			model := New("test-model", WithHTTPClient(client), tc.opt)
			generated, generateErr := model.DoGenerate(context.Background(), provider.CallOptions{})
			streamed, streamErr := model.DoStream(context.Background(), provider.CallOptions{})
			assert.Nil(t, generated)
			assert.Nil(t, streamed)
			for _, err := range []error{generateErr, streamErr} {
				if tc.err != nil {
					assert.ErrorIs(t, err, tc.err)
					assert.ErrorContains(t, err, "transforming request body")
				} else {
					assert.ErrorContains(t, err, "transforming request body returned nil")
				}
			}
			assert.Zero(t, requests)
		})
	}
}

func TestRequestTransformAndWarnings_CoreConsumers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	added := provider.Warning{Type: provider.WarnCompatibility, Feature: "model", Details: "using a backend alias"}
	model := New("test-model", WithBaseURL(server.URL), WithRequestTransformAndWarnings(
		func(body map[string]any, warnings []provider.Warning) (map[string]any, []provider.Warning, error) {
			return body, append(warnings, added), nil
		}))
	for _, streaming := range []bool{false, true} {
		t.Run(strconv.FormatBool(streaming), func(t *testing.T) {
			var warnings []provider.Warning
			var steps []aisdk.StepResult
			if streaming {
				result := aisdk.StreamText(context.Background(), model, aisdk.WithTopK(5), aisdk.WithModelMessages(provider.UserText("hello")))
				for range result.FullStream() {
				}
				require.NoError(t, result.Err())
				assert.Equal(t, "hello", result.Text())
				warnings, steps = result.Warnings(), result.Steps()
			} else {
				result, err := aisdk.GenerateText(context.Background(), model, aisdk.WithTopK(5), aisdk.WithModelMessages(provider.UserText("hello")))
				require.NoError(t, err)
				assert.Equal(t, "hello", result.Text)
				warnings, steps = result.Warnings, result.Steps
			}
			require.Len(t, warnings, 2)
			assert.Equal(t, "topK", warnings[0].Feature)
			assert.Equal(t, added, warnings[1])
			require.Len(t, steps, 1)
			assert.Equal(t, warnings, steps[0].Warnings)
		})
	}
}

func TestRequestTransformAndWarnings_Isolation(t *testing.T) {
	m := &model{modelID: "test-model"}
	WithRequestTransformAndWarnings(func(body map[string]any, warnings []provider.Warning) (map[string]any, []provider.Warning, error) {
		return body, append(warnings, provider.Warning{Type: provider.WarnOther, Message: body["user"].(string)}), nil
	})(m)
	check := func(t *testing.T, id int) {
		t.Helper()
		user := strconv.Itoa(id)
		opts := provider.CallOptions{ProviderOptions: provider.ProviderOptions{"openaiCompatible": OpenAIOptions{User: user}}}
		if id%2 == 0 {
			topK := 5
			opts.TopK = &topK
		}
		body, warnings, err := m.buildRequest(opts, id%3 == 0)
		require.NoError(t, err)
		assert.Equal(t, user, body["user"])
		want := []provider.Warning{{Type: provider.WarnOther, Message: user}}
		if opts.TopK != nil {
			want = append([]provider.Warning{{Type: provider.WarnUnsupported, Feature: "topK", Details: "OpenAI-compatible Chat Completions APIs do not support topK."}}, want...)
		}
		assert.Equal(t, want, warnings)
	}
	t.Run("sequential", func(t *testing.T) {
		for id := range 4 {
			check(t, id)
		}
	})
	t.Run("concurrent", func(t *testing.T) {
		for id := range 24 {
			t.Run(strconv.Itoa(id), func(t *testing.T) {
				t.Parallel()
				check(t, id)
			})
		}
	})
}
