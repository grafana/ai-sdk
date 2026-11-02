package gateway

import (
	"errors"
	"net/http"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/manifestdata"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/service"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

const (
	routeConfig        = "config"
	routeLanguageModel = "language_model"
	routeUnmatched     = "unmatched"
)

var errInvalidRoute = errors.New("gateway: invalid route table")

// route registers one adapter operation. Path is relative to [RoutePrefix].
// Every route gets the access decision and request telemetry under its label,
// and refused callers get errors' permission document.
type route struct {
	method  string
	path    string
	label   string
	handler http.Handler
	errors  host.ErrorWriter
}

// RoutePrefix returns the path prefix of the plugin's namespaced routes,
// ending in the namespace wildcard.
func RoutePrefix() string {
	data := manifestdata.LocalManifest().ManifestData
	return "/apis/" + data.Group + "/" + data.PreferredVersion + "/namespaces/{namespace}"
}

// productionRoutes is the route table the committed manifest declares.
func productionRoutes(discovery, language http.Handler) []route {
	errorWriter := providerv4.NewHostErrorWriter()
	return []route{
		{method: http.MethodGet, path: "/config", label: routeConfig, handler: discovery, errors: errorWriter},
		{method: http.MethodPost, path: "/language-model", label: routeLanguageModel, handler: languageModelPath(language), errors: errorWriter},
	}
}

// newHandler composes the route table. A request that matches no route does
// no access, selection or provider work, and is labeled unmatched.
func newHandler(policy gatewayauth.AccessPolicy, telemetry *service.Telemetry, routes []route) (http.Handler, error) {
	if err := validateRoutes(routes); err != nil {
		return nil, err
	}
	mux, labels, err := registerRoutes(policy, telemetry, routes)
	if err != nil {
		return nil, err
	}
	label := func(request *http.Request) string {
		_, pattern := mux.Handler(request)
		if label, ok := labels[pattern]; ok {
			return label
		}
		return routeUnmatched
	}
	return telemetry.Middleware(label, mux), nil
}

// registerRoutes registers every entry. A pattern the mux refuses, such as one
// that conflicts with another entry, fails as an invalid table.
func registerRoutes(policy gatewayauth.AccessPolicy, telemetry *service.Telemetry, routes []route) (mux *http.ServeMux, labels map[string]string, err error) {
	defer func() {
		if recover() != nil {
			mux, labels, err = nil, nil, errInvalidRoute
		}
	}()
	mux = http.NewServeMux()
	labels = make(map[string]string, len(routes))
	for _, entry := range routes {
		refuse := func(w http.ResponseWriter) { entry.errors.WriteHostError(w, host.ErrorPermission) }
		pattern := entry.method + " " + RoutePrefix() + entry.path
		mux.Handle(pattern, gatewayauth.AccessMiddleware(policy, refuse, telemetry.ObserveAccess, entry.handler))
		labels[pattern] = entry.label
		if entry.method == http.MethodGet {
			mux.HandleFunc(http.MethodHead+" "+RoutePrefix()+entry.path, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Allow", http.MethodGet)
				w.WriteHeader(http.StatusMethodNotAllowed)
			})
		}
	}
	return mux, labels, nil
}

func validateRoutes(routes []route) error {
	seen := make(map[string]struct{}, len(routes))
	for _, entry := range routes {
		if entry.method == "" || entry.method == http.MethodHead || !strings.HasPrefix(entry.path, "/") || strings.HasSuffix(entry.path, "/") || entry.label == "" || entry.label == routeUnmatched || entry.handler == nil || entry.errors == nil {
			return errInvalidRoute
		}
		key := entry.method + " " + entry.path
		if _, duplicate := seen[key]; duplicate {
			return errInvalidRoute
		}
		seen[key] = struct{}{}
	}
	return nil
}

func languageModelPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		cloned := request.Clone(request.Context())
		cloned.URL.Path = providerv4.LanguageModelPath
		cloned.URL.RawPath = ""
		next.ServeHTTP(w, cloned)
	})
}
