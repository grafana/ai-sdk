package service

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

// Readiness owns local liveness/readiness state.
type Readiness struct {
	ready atomic.Bool
}

// Set updates readiness.
func (readiness *Readiness) Set(value bool) { readiness.ready.Store(value) }

// Ready reports current readiness.
func (readiness *Readiness) Ready() bool { return readiness.ready.Load() }

// RouterDependencies contains the required service dependencies.
type RouterDependencies struct {
	Readiness     *Readiness
	Telemetry     *Telemetry
	Authenticator gatewayauth.RequestAuthenticator
	// AuthSource is the configured authentication mode, including for failed requests.
	AuthSource      gatewayauth.Source
	ErrorWriter     *providerv4.HostErrorWriter
	Discovery       http.Handler
	LanguageModel   http.Handler
	ChatCompletions http.Handler
}

// NewRouter combines the two API routes and three operational routes.
func NewRouter(deps RouterDependencies) http.Handler {
	return newRouter(deps, true, true)
}

// NewAPIRouter exposes only the two protected API routes.
func NewAPIRouter(deps RouterDependencies) http.Handler {
	return newRouter(deps, true, false)
}

// NewOperationalRouter exposes only GET /live, /ready, and /metrics.
// Only Readiness and Telemetry are required. Split routers must share Telemetry.
func NewOperationalRouter(deps RouterDependencies) http.Handler {
	return newRouter(deps, false, true)
}

func newRouter(deps RouterDependencies, api, operational bool) http.Handler {
	type route struct {
		method string
		handle http.HandlerFunc
	}
	routes := make(map[string]route)
	if operational {
		routes["/live"] = route{http.MethodGet, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}}
		routes["/ready"] = route{http.MethodGet, func(w http.ResponseWriter, _ *http.Request) {
			if !deps.Readiness.Ready() {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}}
		routes["/metrics"] = route{http.MethodGet, deps.Telemetry.Handler().ServeHTTP}
	}
	if api {
		writeAuthFailure := func(w http.ResponseWriter) {
			deps.ErrorWriter.Write(w, providerv4.HostErrorAuthentication)
		}
		observe := func(ctx context.Context, observation gatewayauth.Observation) {
			observation.Source = deps.AuthSource
			deps.Telemetry.ObserveAuthentication(ctx, observation)
		}
		protectedDiscovery := gatewayauth.Middleware(deps.Authenticator, writeAuthFailure, observe, deps.Discovery)
		protectedLanguageModel := gatewayauth.Middleware(deps.Authenticator, writeAuthFailure, observe, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			cloned := request.Clone(request.Context())
			urlCopy := *request.URL
			urlCopy.Path = providerv4.LanguageModelPath
			urlCopy.RawPath = ""
			cloned.URL = &urlCopy
			deps.LanguageModel.ServeHTTP(w, cloned)
		}))
		routes["/api/v1/aisdk/config"] = route{http.MethodGet, protectedDiscovery.ServeHTTP}
		routes["/api/v1/aisdk/language-model"] = route{http.MethodPost, protectedLanguageModel.ServeHTTP}
		if deps.ChatCompletions != nil {
			protectedAdapter := gatewayauth.Middleware(gatewayauth.AdapterAuthenticator(deps.Authenticator, deps.AuthSource), func(w http.ResponseWriter) { chatcompletions.WriteError(w, 401) }, observe, deps.ChatCompletions)
			routes[chatcompletions.Path] = route{http.MethodPost, protectedAdapter.ServeHTTP}
		}
	}

	dispatch := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if api && (request.URL.Path == "/v1" || strings.HasPrefix(request.URL.Path, "/v1/")) {
			if request.URL.Path != chatcompletions.Path || request.URL.RawPath != "" || deps.ChatCompletions == nil {
				chatcompletions.WriteError(w, 404)
				return
			}
			if request.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				chatcompletions.WriteError(w, 405)
				return
			}
			routes[chatcompletions.Path].handle(w, request)
			return
		}
		if request.URL.RawPath != "" {
			http.NotFound(w, request)
			return
		}
		route, ok := routes[request.URL.Path]
		if !ok {
			http.NotFound(w, request)
			return
		}
		serveMethod(w, request, route.method, route.handle)
	})
	return deps.Telemetry.Middleware(dispatch)
}

func serveMethod(w http.ResponseWriter, request *http.Request, allowed string, next http.HandlerFunc) {
	if request.Method != allowed {
		w.Header().Set("Allow", allowed)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	next(w, request)
}
