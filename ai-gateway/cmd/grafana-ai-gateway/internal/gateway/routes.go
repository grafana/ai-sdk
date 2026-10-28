package gateway

import (
	"net/http"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/manifestdata"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/service"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

const (
	routeConfig        = "config"
	routeLanguageModel = "language_model"
	routeUnmatched     = "unmatched"
)

type routeHandlers struct {
	discovery http.Handler
	language  http.Handler
}

// RoutePrefix returns the path prefix of the plugin's namespaced routes,
// ending in the namespace wildcard.
func RoutePrefix() string {
	data := manifestdata.LocalManifest().ManifestData
	return "/apis/" + data.Group + "/" + data.PreferredVersion + "/namespaces/{namespace}"
}

func newHandler(policy gatewayauth.AccessPolicy, telemetry *service.Telemetry, errorWriter host.ErrorWriter, routes routeHandlers) http.Handler {
	refuse := func(w http.ResponseWriter) { errorWriter.WriteHostError(w, host.ErrorPermission) }
	protect := func(next http.Handler) http.Handler {
		return gatewayauth.AccessMiddleware(policy, refuse, telemetry.ObserveAccess, next)
	}
	configPattern := http.MethodGet + " " + RoutePrefix() + "/config"
	languagePattern := http.MethodPost + " " + RoutePrefix() + "/language-model"
	mux := http.NewServeMux()
	mux.Handle(configPattern, protect(routes.discovery))
	mux.HandleFunc(http.MethodHead+" "+RoutePrefix()+"/config", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.Handle(languagePattern, protect(languageModelPath(routes.language)))
	label := func(request *http.Request) string {
		switch _, pattern := mux.Handler(request); pattern {
		case configPattern:
			return routeConfig
		case languagePattern:
			return routeLanguageModel
		default:
			return routeUnmatched
		}
	}
	return telemetry.Middleware(label, mux)
}

func languageModelPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		cloned := request.Clone(request.Context())
		cloned.URL.Path = providerv4.LanguageModelPath
		cloned.URL.RawPath = ""
		next.ServeHTTP(w, cloned)
	})
}
