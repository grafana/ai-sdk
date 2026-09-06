package service

import (
	"net/http"
	"sync/atomic"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/authlib/authn"
)

// Readiness owns local liveness/readiness state.
type Readiness struct {
	ready atomic.Bool
}

// Set updates readiness.
func (readiness *Readiness) Set(value bool) { readiness.ready.Store(value) }

// Ready reports current readiness.
func (readiness *Readiness) Ready() bool { return readiness.ready.Load() }

// NewRouter constructs the exact five-route service dispatcher.
func NewRouter(readiness *Readiness, telemetry *Telemetry, authenticator authn.Authenticator, errorWriter *providerv4.HostErrorWriter, discovery, languageModel http.Handler) http.Handler {
	protectedDiscovery := gatewayauth.Middleware(authenticator, errorWriter, telemetry.ObserveAuthentication, discovery)
	protectedLanguageModel := gatewayauth.Middleware(authenticator, errorWriter, telemetry.ObserveAuthentication, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		cloned := request.Clone(request.Context())
		urlCopy := *request.URL
		urlCopy.Path = providerv4.LanguageModelPath
		urlCopy.RawPath = ""
		cloned.URL = &urlCopy
		languageModel.ServeHTTP(w, cloned)
	}))

	dispatch := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.RawPath != "" {
			http.NotFound(w, request)
			return
		}
		switch request.URL.Path {
		case "/live":
			serveMethod(w, request, http.MethodGet, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
		case "/ready":
			serveMethod(w, request, http.MethodGet, func(w http.ResponseWriter, _ *http.Request) {
				if !readiness.Ready() {
					http.Error(w, "not ready", http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusOK)
			})
		case "/metrics":
			serveMethod(w, request, http.MethodGet, telemetry.Handler().ServeHTTP)
		case "/api/v1/aisdk/config":
			serveMethod(w, request, http.MethodGet, protectedDiscovery.ServeHTTP)
		case "/api/v1/aisdk/language-model":
			serveMethod(w, request, http.MethodPost, protectedLanguageModel.ServeHTTP)
		default:
			http.NotFound(w, request)
		}
	})
	return telemetry.Middleware(dispatch)
}

func serveMethod(w http.ResponseWriter, request *http.Request, allowed string, next http.HandlerFunc) {
	if request.Method != allowed {
		w.Header().Set("Allow", allowed)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	next(w, request)
}
