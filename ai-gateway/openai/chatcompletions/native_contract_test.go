package chatcompletions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeContract_Regression(t *testing.T) {
	t.Run("duplicate last value", func(t *testing.T) {
		r, err := mapRequest([]byte(strings.TrimSuffix(basic, "}") + `,"model":"canonical"}`))
		require.NoError(t, err)
		assert.Equal(t, "canonical", r.Model)
	})
	t.Run("native identity and warnings", func(t *testing.T) {
		result := textResult()
		result.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "native-id", ModelID: "native-model", Timestamp: time.Unix(123, 0)}}
		result.Warnings = []provider.Warning{{Type: provider.WarnUnsupported, Feature: "seed", Details: "ignored"}}
		h := testHandler(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }}, nil)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", Path, strings.NewReader(basic))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		require.Equal(t, 200, rr.Code, rr.Body.String())
		var body map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		assert.Equal(t, "native-id", body["id"])
		assert.Equal(t, "native-model", body["model"])
		assert.EqualValues(t, 123, body["created"])
		require.Contains(t, body, "grafana")
	})
}

func TestNativeContract_SchemaAndDefaults(t *testing.T) {
	for _, raw := range []string{
		`{"model":"alias","messages":[{"role":"user","content":"hello"}],"response_format":{"type":"json_schema","json_schema":{"name":"x","schema":{"type":"object"}}},"response_format":{"type":"text"}}`,
		`{"model":"alias","messages":[{"role":"user","content":"hello"}],"stop":"a","stop":["b"]}`,
	} {
		_, err := mapRequest([]byte(raw))
		require.NoError(t, err)
	}
	for _, extra := range []string{`,"reasoning_effort":"invented"`, `,"tools":[{"type":"function","function":{"name":"f","parameters":{},"Strict":true}}]`, `,"messages":[{"role":"user","content":[{"type":"text","Text":"x"}]}]`} {
		_, err := mapRequest([]byte(strings.TrimSuffix(basic, "}") + extra + "}"))
		require.Error(t, err)
	}
	for _, tc := range []struct {
		backend   Backend
		namespace string
		want      string
	}{
		{BackendResponses, "", `{"openai":{"store":false,"strictJsonSchema":false,"parallelToolCalls":false}}`},
		{BackendCompatible, "custom-vendor", `{"custom-vendor":{"store":false,"strictJsonSchema":false,"parallel_tool_calls":false}}`},
		{BackendCompatible, "openai", `{"openai":{"store":false,"strictJsonSchema":false,"parallel_tool_calls":false}}`},
		{BackendAnthropic, "", `{"anthropic":{"disableParallelToolUse":true}}`},
	} {
		t.Run(string(tc.backend)+tc.namespace, func(t *testing.T) {
			r := requestWith(t, `,"parallel_tool_calls":false`)
			m := defaultsModel{backend: tc.backend, namespace: tc.namespace}
			options, err := m.options(context.WithValue(context.Background(), defaultsKey{}, requestDefaults{strict: r.strictOutput, parallel: r.Parallel}), r.options)
			require.NoError(t, err)
			encoded, err := json.Marshal(options.ProviderOptions)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
			assert.Nil(t, r.options.ProviderOptions)
			unchanged, err := m.options(context.Background(), r.options)
			require.NoError(t, err)
			assert.Nil(t, unchanged.ProviderOptions)
		})
	}
}

func TestNativeContract_StreamingSDK(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprint(late), func(t *testing.T) {
			parts := textParts()
			parts[0].Warnings = []provider.Warning{{Type: provider.WarnUnsupported, Feature: "seed", Details: "ignored <seed>"}}
			meta := provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "native-id", ModelID: "native-model", Timestamp: time.Unix(123, 0)}
			index := 1
			if late {
				index = 3
			}
			parts = append(parts[:index], append([]provider.StreamPart{meta}, parts[index:]...)...)
			server := httptest.NewServer(testHandler(t, &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return partsStream(parts...), nil
			}}, nil))
			defer server.Close()
			client := openaisdk.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("dummy"), option.WithMaxRetries(0))
			stream := client.Chat.Completions.NewStreaming(context.Background(), openaisdk.ChatCompletionNewParams{Model: "alias", Messages: []openaisdk.ChatCompletionMessageParamUnion{openaisdk.UserMessage("hello")}})
			defer func() { require.NoError(t, stream.Close()) }()
			id, model := "", ""
			var created int64
			warningsSeen, nativeSeen := false, false
			for stream.Next() {
				c := stream.Current()
				if id == "" {
					id, model, created = c.ID, c.Model, c.Created
				}
				assert.Equal(t, id, c.ID)
				assert.Equal(t, model, c.Model)
				assert.Equal(t, created, c.Created)
				if field, ok := c.JSON.ExtraFields["grafana"]; ok {
					var extension diagnostics
					require.NoError(t, json.Unmarshal([]byte(field.Raw()), &extension))
					if len(extension.Warnings) > 0 {
						warningsSeen = true
						assert.Equal(t, "ignored <seed>", extension.Warnings[0].Details)
					}
					if extension.NativeResponse != nil {
						nativeSeen = true
						assert.Equal(t, "native-id", extension.NativeResponse.ID)
					}
				}
			}
			require.NoError(t, stream.Err())
			assert.True(t, warningsSeen)
			if late {
				assert.True(t, nativeSeen)
				assert.NotEqual(t, "native-id", id)
			} else {
				assert.Equal(t, "native-id", id)
				assert.Equal(t, "native-model", model)
				assert.Equal(t, int64(123), created)
			}
		})
	}
}

func TestNativeContract_DiagnosticBounds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		warnings []provider.Warning
		response *provider.GenerateResponse
	}{
		{name: "oversized warning", warnings: []provider.Warning{{Type: provider.WarnOther, Message: strings.Repeat("x", 2048)}}},
		{name: "invalid warning", warnings: []provider.Warning{{Type: provider.WarnOther, Message: string([]byte{255})}}},
		{name: "oversized identity", response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: strings.Repeat("x", 2048)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := textResult()
			result.Warnings = tc.warnings
			result.Response = tc.response
			_, err := mapGenerate(result, requestWith(t, ""), "id", "model", 1, 1024)
			require.Error(t, err)
		})
	}
}
