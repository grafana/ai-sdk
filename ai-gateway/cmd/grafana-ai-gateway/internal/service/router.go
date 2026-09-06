package service

import (
	"fmt"
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

// RouterConfig configures the exact service HTTP surface.
type RouterConfig struct {
	Readiness     *Readiness
	Telemetry     *Telemetry
	Authenticator authn.Authenticator
	ErrorWriter   *providerv4.HostErrorWriter
	Discovery     http.Handler
	LanguageModel http.Handler
}

// NewRouter constructs the exact five-route service dispatcher.
func NewRouter(config RouterConfig) (http.Handler, error) {
	if config.Readiness == nil || config.Telemetry == nil || config.Discovery == nil || config.LanguageModel == nil {
		return nil, fmt.Errorf("gateway service: router dependency is nil")
	}
	protectedDiscovery, err := gatewayauth.Middleware(config.Authenticator, config.ErrorWriter, config.Telemetry.ObserveAuthentication, config.Discovery)
	if err != nil {
		return nil, err
	}
	protectedLanguageModel, err := gatewayauth.Middleware(config.Authenticator, config.ErrorWriter, config.Telemetry.ObserveAuthentication, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		cloned := request.Clone(request.Context())
		urlCopy := *request.URL
		urlCopy.Path = providerv4.LanguageModelPath
		urlCopy.RawPath = ""
		cloned.URL = &urlCopy
		config.LanguageModel.ServeHTTP(w, cloned)
	}))
	if err != nil {
		return nil, err
	}

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
				if !config.Readiness.Ready() {
					http.Error(w, "not ready", http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusOK)
			})
		case "/metrics":
			serveMethod(w, request, http.MethodGet, config.Telemetry.Handler().ServeHTTP)
		case "/api/v1/aisdk/config":
			serveMethod(w, request, http.MethodGet, protectedDiscovery.ServeHTTP)
		case "/api/v1/aisdk/language-model":
			serveMethod(w, request, http.MethodPost, protectedLanguageModel.ServeHTTP)
		default:
			http.NotFound(w, request)
		}
	})
	return config.Telemetry.Middleware(dispatch), nil
}

func serveMethod(w http.ResponseWriter, request *http.Request, allowed string, next http.HandlerFunc) {
	if request.Method != allowed {
		w.Header().Set("Allow", allowed)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	next(w, request)
}
