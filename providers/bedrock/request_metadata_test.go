package bedrock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequest_RequestMetadataRejectsInvalidValues(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	defer server.Close()
	model := New(testAnthropicModel, WithBaseURL(server.URL), WithBearerToken("test-token"))
	for _, namespace := range providerOptionKeys {
		for _, tc := range []struct {
			name     string
			metadata string
		}{
			{name: "number entry", metadata: `{"private-key":42}`},
			{name: "boolean entry", metadata: `{"private-key":true}`},
			{name: "null entry", metadata: `{"private-key":null}`},
			{name: "object entry", metadata: `{"private-key":{"private-value":"secret"}}`},
			{name: "array entry", metadata: `{"private-key":["private-value"]}`},
			{name: "null map", metadata: `null`},
			{name: "array map", metadata: `[]`},
			{name: "string map", metadata: `"private-value"`},
			{name: "number map", metadata: `42`},
			{name: "boolean map", metadata: `false`},
		} {
			t.Run(namespace+"/"+tc.name, func(t *testing.T) {
				opts := provider.CallOptions{
					Prompt: []provider.Message{provider.UserText("hi")},
					ProviderOptions: provider.ProviderOptions{namespace: provider.RawProviderOption{
						Key: namespace, Raw: json.RawMessage(`{"requestMetadata":` + tc.metadata + `}`),
					}},
				}
				if namespace == "amazonBedrock" {
					opts.ProviderOptions["bedrock"] = BedrockOptions{RequestMetadata: map[string]string{"valid": "legacy"}}
				}
				_, _, _, buildErr := buildRequest(testAnthropicModel, opts)
				_, generateErr := model.DoGenerate(context.Background(), opts)
				_, streamErr := model.DoStream(context.Background(), opts)
				for _, err := range []error{buildErr, generateErr, streamErr} {
					require.Error(t, err)
					assert.Contains(t, err.Error(), "requestMetadata")
					assert.NotContains(t, err.Error(), "private-key")
					assert.NotContains(t, err.Error(), "private-value")
				}
			})
		}
	}
	assert.Zero(t, requests.Load(), "invalid metadata must fail before HTTP")
}

func TestConverseRequestMetadata(t *testing.T) {
	for _, namespace := range providerOptionKeys {
		for _, form := range []string{"typed", "pointer", "raw", "roundtrip"} {
			for _, tc := range []struct {
				name     string
				metadata map[string]string
			}{
				{name: "omitted"},
				{name: "empty", metadata: map[string]string{}},
				{name: "populated", metadata: map[string]string{"team": "request-only-value", "équipe": "東京", "": ""}},
			} {
				for _, streaming := range []bool{false, true} {
					mode := "generate"
					if streaming {
						mode = "stream"
					}
					t.Run(namespace+"/"+form+"/"+tc.name+"/"+mode, func(t *testing.T) {
						captured := make(chan []byte, 1)
						server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							body, err := io.ReadAll(r.Body)
							require.NoError(t, err)
							captured <- body
							if streaming {
								assert.True(t, strings.HasSuffix(r.URL.Path, "/converse-stream"))
								w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
								_, err = w.Write(encodeFixtures(t, `{"messageStart":{"role":"assistant"}}`, `{"messageStop":{"stopReason":"end_turn"}}`, `{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`))
							} else {
								assert.True(t, strings.HasSuffix(r.URL.Path, "/converse"))
								w.Header().Set("Content-Type", "application/json")
								_, err = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
							}
							require.NoError(t, err)
						}))
						defer server.Close()
						typed := BedrockOptions{RequestMetadata: tc.metadata}
						var option provider.ProviderOption = typed
						switch form {
						case "pointer":
							option = &typed
						case "raw":
							raw := []byte(`{}`)
							if tc.metadata != nil {
								var err error
								raw, err = json.Marshal(map[string]any{"requestMetadata": tc.metadata})
								require.NoError(t, err)
							}
							option = provider.RawProviderOption{Key: namespace, Raw: raw}
						}
						opts := provider.CallOptions{
							Prompt:          []provider.Message{provider.UserText("hi")},
							ProviderOptions: provider.ProviderOptions{namespace: option},
						}
						if form == "roundtrip" {
							encoded, err := json.Marshal(opts.ProviderOptions)
							require.NoError(t, err)
							require.NoError(t, json.Unmarshal(encoded, &opts.ProviderOptions))
						}
						model := New(testAnthropicModel, WithBaseURL(server.URL), WithBearerToken("test-token"))
						if streaming {
							result, err := model.DoStream(context.Background(), opts)
							require.NoError(t, err)
							for part := range result.Stream {
								assert.NotEqual(t, provider.PartError, part.Type)
								data, err := json.Marshal(part)
								require.NoError(t, err)
								assert.NotContains(t, string(data), "request-only-value")
							}
						} else {
							result, err := model.DoGenerate(context.Background(), opts)
							require.NoError(t, err)
							data, err := json.Marshal(result.ProviderMetadata)
							require.NoError(t, err)
							assert.NotContains(t, string(data), "request-only-value")
							assert.Empty(t, result.Warnings)
						}
						var request map[string]json.RawMessage
						require.NoError(t, json.Unmarshal(<-captured, &request))
						if tc.metadata == nil {
							assert.NotContains(t, request, "requestMetadata")
						} else {
							want, err := json.Marshal(tc.metadata)
							require.NoError(t, err)
							assert.JSONEq(t, string(want), string(request["requestMetadata"]))
						}
						assert.NotContains(t, string(request["additionalModelRequestFields"]), "requestMetadata")
					})
				}
			}
		}
	}
}

func TestBuildRequest_RequestMetadataNamespacePrecedence(t *testing.T) {
	raw := func(value string) provider.ProviderOption {
		return provider.RawProviderOption{Raw: json.RawMessage(value)}
	}
	for _, tc := range []struct {
		name   string
		modern provider.ProviderOption
		legacy provider.ProviderOption
		want   string
	}{
		{name: "absent modern", legacy: raw(`{"requestMetadata":{"source":"legacy"}}`), want: `{"source":"legacy"}`},
		{name: "null modern", modern: raw(`null`), legacy: raw(`{"requestMetadata":{"source":"legacy"}}`), want: `{"source":"legacy"}`},
		{name: "nil pointer modern", modern: (*BedrockOptions)(nil), legacy: raw(`{"requestMetadata":{"source":"legacy"}}`), want: `{"source":"legacy"}`},
		{name: "empty modern object", modern: raw(`{}`), legacy: raw(`{"requestMetadata":{"source":"legacy"}}`)},
		{name: "empty typed modern", modern: BedrockOptions{}, legacy: raw(`{"requestMetadata":{"source":"legacy"}}`)},
		{name: "empty modern metadata", modern: raw(`{"requestMetadata":{}}`), legacy: raw(`{"requestMetadata":{"source":"legacy"}}`), want: `{}`},
		{name: "modern replaces legacy", modern: raw(`{"requestMetadata":{"source":"modern"}}`), legacy: raw(`{"requestMetadata":{"other":"legacy"}}`), want: `{"source":"modern"}`},
		{name: "malformed legacy ignored", modern: raw(`{"requestMetadata":{"source":"modern"}}`), legacy: raw(`{"requestMetadata":null}`), want: `{"source":"modern"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
				Prompt:          []provider.Message{provider.UserText("hi")},
				ProviderOptions: provider.ProviderOptions{"amazonBedrock": tc.modern, "bedrock": tc.legacy},
			})
			require.NoError(t, err)
			body, err := json.Marshal(req)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &fields))
			if tc.want == "" {
				assert.NotContains(t, fields, "requestMetadata")
			} else {
				assert.JSONEq(t, tc.want, string(fields["requestMetadata"]))
			}
		})
	}
}

func TestBedrockOptions_RequestMetadataJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want map[string]string
	}{
		{name: "ordinary field matching", raw: `{"RequestMetadata":{"team":"search"}}`, want: map[string]string{"team": "search"}},
		{name: "duplicate entries", raw: `{"requestMetadata":{"team":"old","team":"new"}}`, want: map[string]string{"team": "new"}},
		{name: "last duplicate value wins", raw: `{"requestMetadata":{"team":42,"team":"new"}}`, want: map[string]string{"team": "new"}},
		{name: "last valid field wins", raw: `{"requestMetadata":null,"requestMetadata":{"team":"new"}}`, want: map[string]string{"team": "new"}},
		{name: "last field spelling wins", raw: `{"requestMetadata":{"team":"search"},"RequestMetadata":{"environment":"prod"}}`, want: map[string]string{"environment": "prod"}},
		{name: "last canonical field wins", raw: `{"RequestMetadata":{"team":"search"},"requestMetadata":{"environment":"prod"}}`, want: map[string]string{"environment": "prod"}},
		{name: "last duplicate field wins", raw: `{"requestMetadata":{"team":"search"},"requestMetadata":{"environment":"prod"}}`, want: map[string]string{"environment": "prod"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var options BedrockOptions
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &options))
			assert.Equal(t, tc.want, options.RequestMetadata)
			encoded, err := json.Marshal(options)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &fields))
			assert.Contains(t, fields, "requestMetadata")
			assert.NotContains(t, fields, "RequestMetadata")
			req, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
				Prompt:          []provider.Message{provider.UserText("hi")},
				ProviderOptions: provider.ProviderOptions{"amazonBedrock": provider.RawProviderOption{Raw: json.RawMessage(tc.raw)}},
			})
			require.NoError(t, err)
			body, err := json.Marshal(req)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &fields))
			assert.NotContains(t, fields, "RequestMetadata")
			want, err := json.Marshal(tc.want)
			require.NoError(t, err)
			assert.JSONEq(t, string(want), string(fields["requestMetadata"]))
		})
	}
	var options BedrockOptions
	require.ErrorContains(t, json.Unmarshal([]byte(`{"RequestMetadata":{"private-key":null}}`), &options), "requestMetadata")
}

func TestBuildRequest_RequestMetadataDoesNotImposeServiceLimits(t *testing.T) {
	metadata := make(map[string]string)
	for i := range 17 {
		metadata[strings.Repeat("k", 257)+string(rune('a'+i))] = strings.Repeat("v", 257)
	}
	for _, raw := range []bool{false, true} {
		var option provider.ProviderOption = BedrockOptions{RequestMetadata: metadata}
		if raw {
			data, err := json.Marshal(option)
			require.NoError(t, err)
			option = provider.RawProviderOption{Raw: data}
		}
		req, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
			Prompt:          []provider.Message{provider.UserText("hi")},
			ProviderOptions: provider.ProviderOptions{"amazonBedrock": option},
		})
		require.NoError(t, err)
		assert.Equal(t, metadata, req.RequestMetadata)
	}
}
