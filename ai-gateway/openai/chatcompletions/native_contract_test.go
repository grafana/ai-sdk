package chatcompletions

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
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
		h := testHandler(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }})
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
		backend   execution.TargetType
		namespace string
		want      string
	}{
		{execution.TargetOpenAI, "", `{"openai":{"store":false,"strictJsonSchema":false,"parallelToolCalls":false}}`},
		{execution.TargetOpenAICompatible, "custom-vendor", `{"custom-vendor":{"store":false,"strictJsonSchema":false,"parallel_tool_calls":false}}`},
		{execution.TargetOpenAICompatible, "openai", `{"openai":{"store":false,"strictJsonSchema":false,"parallel_tool_calls":false}}`},
		{execution.TargetAnthropic, "", `{"anthropic":{"disableParallelToolUse":true}}`},
	} {
		t.Run(string(tc.backend)+tc.namespace, func(t *testing.T) {
			r := requestWith(t, `,"parallel_tool_calls":false`)
			target := execution.Target{Type: tc.backend, ProviderName: tc.namespace}
			options, err := chatTranslator(r)(target, r.options)
			require.NoError(t, err)
			encoded, err := json.Marshal(options.ProviderOptions)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
			assert.Nil(t, r.options.ProviderOptions)
		})
	}
	t.Run("unknown native target", func(t *testing.T) {
		r := requestWith(t, "")
		_, err := chatTranslator(r)(execution.Target{Type: "unknown"}, r.options)
		require.ErrorIs(t, err, execution.ErrTranslation)
	})
}
