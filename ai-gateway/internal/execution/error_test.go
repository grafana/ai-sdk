package execution

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarize_Fields(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
	}{
		{name: "bare", data: `{"message":"overloaded","type":"capacity","code":900719925474099312345}`},
		{name: "envelope", data: `{"type":"error","error":{"message":"overloaded","type":"capacity","code":900719925474099312345}}`},
		{name: "duplicates", data: `{"message":"discarded secret","\u006dessage":"overloaded","type":"capacity","code":900719925474099312345}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := provider.NewAPICallError(provider.APICallErrorOptions{Message: "API fallback message", URL: "https://ordinary.example/models", Cause: errors.New("cause stays private"), StatusCode: 503, Data: json.RawMessage(tc.data)})
			before := append(json.RawMessage(nil), api.Data...)
			got := summarize(fmt.Errorf("candidate context: %w", api), []string{"discarded secret"}, 4096)
			require.NotNil(t, got)
			assert.Equal(t, "overloaded", got.Message)
			assert.Equal(t, "capacity", got.Type)
			assert.Equal(t, json.RawMessage(`900719925474099312345`), got.Code)
			assert.Equal(t, 503, got.StatusCode)
			assert.Equal(t, before, api.Data)
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			for _, absent := range []string{"cause stays private", "ordinary.example", "discarded secret", "details", "isRetryable"} {
				assert.NotContains(t, string(encoded), absent)
			}
		})
	}
}

func TestSummarize_GracefulDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		data   string
		limit  int64
		status int
	}{
		{name: "malformed", data: `{"message":"partial`, limit: 4096, status: 503},
		{name: "over read bound", data: `{"message":"secret"}`, limit: 4, status: 429},
		{name: "invalid UTF-8", data: "{\"message\":\"\xff\"}", limit: 4096, status: 400},
		{name: "array", data: `[{"message":"unsafe"}]`, limit: 4096, status: 500},
		{name: "invalid nested error", data: `{"error":[{"message":"unsafe"}]}`, limit: 4096, status: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := provider.NewAPICallError(provider.APICallErrorOptions{Message: "do not reuse malformed body", Data: json.RawMessage(tc.data), StatusCode: tc.status})
			got := summarize(api, nil, tc.limit)
			require.NotNil(t, got)
			assert.Equal(t, &Failure{StatusCode: tc.status}, got)
		})
	}
	for _, responseBody := range []bool{false, true} {
		api := provider.NewAPICallError(provider.APICallErrorOptions{Message: "native message", URL: "https://ordinary.example", Cause: errors.New("private cause"), StatusCode: 503})
		if responseBody {
			api.ResponseBody = `{"message":"body fallback"}`
		}
		got := summarize(api, nil, 4096)
		require.NotNil(t, got)
		want := "native message"
		if responseBody {
			want = "body fallback"
		}
		assert.Equal(t, want, got.Message)
	}
	assert.Nil(t, summarize(errors.Join(errors.New("first"), errors.New("second")), nil, 4096))
	assert.Nil(t, summarize(fmt.Errorf("wrapped: %w", errors.Join(errors.New("first"), errors.New("second"))), nil, 4096))
}

func TestSummarize_Protection(t *testing.T) {
	for _, secret := range []string{"dummy<key>", "dummy&key", `dummy"key`, `dummy\key`} {
		t.Run(secret, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"message": "safe", "type": secret, "code": secret})
			require.NoError(t, err)
			got := summarize(provider.NewAPICallError(provider.APICallErrorOptions{Data: body}), []string{secret}, 4096)
			require.NotNil(t, got)
			assert.Equal(t, "safe", got.Message)
			assert.Empty(t, got.Type)
			assert.Nil(t, got.Code)
		})
	}
	for _, body := range []string{
		`{"apiKey":"dummy-key","error":{"message":"echo dummy-key","type":"capacity"}}`,
		`{"error":{"tenantId":"other-tenant","message":"failure for other-tenant","type":"capacity"}}`,
		`{"Authorization":"Bearer dummy-key","message":"echo dummy-key","type":"capacity"}`,
		`{"apiKey":"old","apiKey":"dummy-key","message":"echo dummy-key","type":"capacity"}`,
	} {
		got := summarize(provider.NewAPICallError(provider.APICallErrorOptions{Data: json.RawMessage(body)}), nil, 4096)
		require.NotNil(t, got)
		assert.Empty(t, got.Message)
		assert.Equal(t, "capacity", got.Type)
	}
	got := summarize(provider.NewAPICallError(provider.APICallErrorOptions{URL: "https://user:dummy-key@ordinary.example?api_key=query-key", Message: "echo query-key dummy-key", StatusCode: 503}), nil, 4096)
	require.NotNil(t, got)
	assert.Empty(t, got.Message)
	cause := &url.Error{Op: "POST", URL: "https://user:dummy-key@ordinary.example?api_key=query-key", Err: errors.New("connection failed")}
	got = summarize(provider.NewAPICallError(provider.APICallErrorOptions{Message: cause.Error(), Cause: cause, StatusCode: 503}), nil, 4096)
	require.NotNil(t, got)
	assert.Empty(t, got.Message)
	assert.Equal(t, 503, got.StatusCode)
	plain := summarize(errors.New("ordinary network failure"), nil, 4096)
	assert.Equal(t, &Failure{Message: "ordinary network failure"}, plain)
	assert.Nil(t, summarize(errors.New("echo dummy-key"), []string{"dummy-key"}, 4096))
}

func TestSummarize_OuterFieldsNotInherited(t *testing.T) {
	api := provider.NewAPICallError(provider.APICallErrorOptions{Message: "native message", Data: json.RawMessage(`{"type":"error","message":"outer message","error":{"code":"inner"}}`)})
	got := summarize(api, nil, 4096)
	require.NotNil(t, got)
	assert.Empty(t, got.Type)
	assert.Equal(t, "native message", got.Message)
	assert.Equal(t, json.RawMessage(`"inner"`), got.Code)
}

func TestSummarize_UsefulApplicationData(t *testing.T) {
	message := "model sk-application-data at https://ordinary.example; token=ordinary"
	body, err := json.Marshal(map[string]any{"message": message, "code": "", "unknown": map[string]string{"privateKey": "not republished", "request": "not republished"}})
	require.NoError(t, err)
	got := summarize(provider.NewAPICallError(provider.APICallErrorOptions{Data: body}), nil, 4096)
	require.NotNil(t, got)
	assert.Equal(t, message, got.Message)
	assert.Equal(t, json.RawMessage(`""`), got.Code)
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "not republished")
	long := strings.Repeat("x", 40<<10)
	got = summarize(provider.NewAPICallError(provider.APICallErrorOptions{Message: long}), nil, 64<<10)
	require.NotNil(t, got)
	assert.Equal(t, long, got.Message)
}
