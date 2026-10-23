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
		{name: "additive fields", data: `{"message":"overloaded","type":"capacity","code":900719925474099312345,"future":{"apiKey":"overloaded"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := provider.NewAPICallError(provider.APICallErrorOptions{Message: "API fallback message", URL: "https://ordinary.example/models", Cause: errors.New("cause stays private"), StatusCode: 503, Data: json.RawMessage(tc.data)})
			before := append(json.RawMessage(nil), api.Data...)
			got := summarize(fmt.Errorf("candidate context: %w", api))
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
		status int
	}{
		{name: "malformed", data: `{"message":"partial`, status: 503},
		{name: "array", data: `[{"message":"unsafe"}]`, status: 500},
		{name: "invalid nested error", data: `{"error":[{"message":"unsafe"}]}`, status: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := provider.NewAPICallError(provider.APICallErrorOptions{Message: "do not reuse malformed body", Data: json.RawMessage(tc.data), StatusCode: tc.status})
			got := summarize(api)
			require.NotNil(t, got)
			assert.Equal(t, &Failure{StatusCode: tc.status}, got)
		})
	}
	for _, responseBody := range []bool{false, true} {
		api := provider.NewAPICallError(provider.APICallErrorOptions{Message: "native message", URL: "https://ordinary.example", Cause: errors.New("private cause"), StatusCode: 503})
		if responseBody {
			api.ResponseBody = `{"message":"body fallback"}`
		}
		got := summarize(api)
		require.NotNil(t, got)
		want := "native message"
		if responseBody {
			want = "body fallback"
		}
		assert.Equal(t, want, got.Message)
	}
	assert.Nil(t, summarize(errors.Join(errors.New("first"), errors.New("second"))))
	assert.Nil(t, summarize(fmt.Errorf("wrapped: %w", errors.Join(errors.New("first"), errors.New("second")))))
}

func TestSummarize_ProviderEchoes(t *testing.T) {
	for _, credential := range []string{"dummy<key>", "dummy&key", `dummy"key`, `dummy\key`, "other-tenant", "900719925474099312345"} {
		t.Run(credential, func(t *testing.T) {
			for _, value := range []string{credential, url.QueryEscape(credential), url.PathEscape(credential)} {
				body, err := json.Marshal(map[string]any{
					"apiKey": credential,
					"error":  map[string]string{"message": "echo " + value, "type": value, "code": value, "tenantId": credential},
				})
				require.NoError(t, err)
				api := provider.NewAPICallError(provider.APICallErrorOptions{
					Data: body, StatusCode: 503, URL: "https://user:" + url.QueryEscape(credential) + "@ordinary.example",
					ResponseHeaders: map[string][]string{"Authorization": {"Bearer " + credential}},
				})
				got := summarize(api)
				require.NotNil(t, got)
				assert.Equal(t, "echo "+value, got.Message)
				assert.Equal(t, value, got.Type)
				var code string
				require.NoError(t, json.Unmarshal(got.Code, &code))
				assert.Equal(t, value, code)
				assert.Equal(t, 503, got.StatusCode)
				assert.Equal(t, json.RawMessage(body), api.Data)
			}
		})
	}
	for _, body := range []string{
		`{"apiKey":"dummy-key","error":{"message":"echo dummy-key","type":"capacity"}}`,
		`{"Authorization":"Bearer dummy-key","message":"echo dummy-key","type":"capacity"}`,
		`{"apiKey":"old","apiKey":"dummy-key","message":"echo dummy-key","type":"capacity"}`,
	} {
		got := summarize(provider.NewAPICallError(provider.APICallErrorOptions{Data: json.RawMessage(body)}))
		require.NotNil(t, got)
		assert.Equal(t, "echo dummy-key", got.Message)
		assert.Equal(t, "capacity", got.Type)
	}
	cause := &url.Error{Op: "POST", URL: "https://user:dummy-key@ordinary.example?api_key=query-key", Err: errors.New("connection failed")}
	got := summarize(provider.NewAPICallError(provider.APICallErrorOptions{Message: cause.Error(), Cause: cause, StatusCode: 503}))
	require.NotNil(t, got)
	assert.Equal(t, cause.Error(), got.Message)
	assert.Equal(t, 503, got.StatusCode)
	assert.Equal(t, &Failure{Message: "echo dummy-key"}, summarize(errors.New("echo dummy-key")))
}

func TestSummarize_StandardStringDecoding(t *testing.T) {
	for _, data := range []string{"{\"message\":\"\xff\"}", `{"message":"\ud800"}`} {
		api := provider.NewAPICallError(provider.APICallErrorOptions{Data: json.RawMessage(data)})
		assert.Equal(t, &Failure{Message: "\ufffd"}, summarize(api))
	}
}

func TestSummarize_OuterFieldsNotInherited(t *testing.T) {
	api := provider.NewAPICallError(provider.APICallErrorOptions{Message: "native message", Data: json.RawMessage(`{"type":"error","message":"outer message","error":{"code":"inner"}}`)})
	got := summarize(api)
	require.NotNil(t, got)
	assert.Empty(t, got.Type)
	assert.Equal(t, "native message", got.Message)
	assert.Equal(t, json.RawMessage(`"inner"`), got.Code)
}

func TestSummarize_UsefulApplicationData(t *testing.T) {
	message := "model sk-application-data at https://ordinary.example; token=ordinary"
	body, err := json.Marshal(map[string]any{"message": message, "code": "", "unknown": map[string]string{"privateKey": "not republished", "request": "not republished"}})
	require.NoError(t, err)
	got := summarize(provider.NewAPICallError(provider.APICallErrorOptions{Data: body}))
	require.NotNil(t, got)
	assert.Equal(t, message, got.Message)
	assert.Equal(t, json.RawMessage(`""`), got.Code)
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "not republished")
}

func TestSummarize_NoSourceLimit(t *testing.T) {
	long := strings.Repeat("x", 8<<20+1)
	for _, tc := range []struct {
		name string
		api  *provider.APICallError
	}{
		{name: "message", api: provider.NewAPICallError(provider.APICallErrorOptions{Message: long, StatusCode: 503})},
		{name: "data", api: provider.NewAPICallError(provider.APICallErrorOptions{Data: json.RawMessage(`{"message":"` + long + `","type":"capacity","code":"busy"}`), StatusCode: 503})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Summary(tc.api)
			require.NotNil(t, got)
			assert.Equal(t, long, got.Message)
			assert.Equal(t, 503, got.StatusCode)
			if tc.name == "data" {
				assert.Equal(t, "capacity", got.Type)
				assert.Equal(t, json.RawMessage(`"busy"`), got.Code)
			}
		})
	}
	api := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 502})
	api.ResponseBody = `{"error":{"message":"` + long + `"}}`
	got := Summary(api)
	require.NotNil(t, got)
	assert.Equal(t, long, got.Message)
}
