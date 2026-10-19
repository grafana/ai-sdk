package service

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsRouterOwnsErrorsBeforeBody(t *testing.T) {
	for _, source := range []gatewayauth.Source{gatewayauth.SourceAccessToken, gatewayauth.SourceCloudGateway} {
		t.Run(string(source), func(t *testing.T) {
			telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
			require.NoError(t, err)
			calls := 0
			authenticator := gatewayauth.NewAccessTokenAuthenticator(&serviceAuthenticator{info: serviceAuthInfo()})
			if source == gatewayauth.SourceCloudGateway {
				authenticator = gatewayauth.NewCloudProviderWireAuthenticator()
			}
			handler := NewRouter(RouterDependencies{Readiness: &Readiness{}, Telemetry: telemetry, Authenticator: authenticator, AuthSource: source, ErrorWriter: providerv4.NewHostErrorWriter(), ChatCompletions: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				_, ok := gatewayauth.CallerFromContext(r.Context())
				assert.True(t, ok)
				w.WriteHeader(204)
			})})
			invalid := []http.Header{{}, {"Authorization": {"Bearer a,b"}}, {"Authorization": {"Bearer one", "Bearer two"}}, {"Authorization": {"Bearer one"}, "authorization": {"Bearer two"}}, {"Authorization": {"Bearer one"}, "X-Access-Token": {"two"}}, {"Authorization": {"Bearer one"}, "X-Grafana-Id": {"two"}}, {"Authorization": {"Bearer one"}, "X-Scope-OrgID": {"42"}}}
			for _, headers := range invalid {
				body := &rejectReadBody{}
				req := httptest.NewRequest("POST", chatcompletions.Path, body)
				req.Header = headers
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)
				assert.Equal(t, 401, w.Code)
				assert.Equal(t, `{"error":{"message":"authentication failed","type":"authentication_error","param":null,"code":"invalid_api_key"}}`, w.Body.String())
				assert.Zero(t, body.reads)
			}
			assert.Zero(t, calls)
			for _, tc := range []struct {
				method, path string
				status       int
			}{{"GET", chatcompletions.Path, 405}, {"POST", "/v1/models", 404}, {"POST", "/v1/chat%2Fcompletions", 404}} {
				body := &rejectReadBody{}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, body))
				assert.Equal(t, tc.status, w.Code)
				assert.Contains(t, w.Body.String(), `"error"`)
				assert.Zero(t, body.reads)
				if tc.status == 405 {
					assert.Equal(t, "POST", w.Header().Get("Allow"))
				}
			}
			req := httptest.NewRequest("POST", chatcompletions.Path, nil)
			if source == gatewayauth.SourceCloudGateway {
				req.Header.Set("X-Scope-OrgID", "42")
			} else {
				req.Header.Set("Authorization", "Bearer access")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t, 204, w.Code)
			assert.Equal(t, 1, calls)
			assert.Contains(t, telemetryMetrics(telemetry), `route="chat_completions"`)
		})
	}
}
