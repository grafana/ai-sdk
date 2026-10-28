package gateway

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/kube-openapi/pkg/spec3"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/manifestdata"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/service"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

func newTestTelemetry(t *testing.T) *service.Telemetry {
	t.Helper()
	telemetry, err := service.NewTelemetry(slog.New(slog.DiscardHandler), service.TelemetryOptions{Registerer: prometheus.NewRegistry()})
	require.NoError(t, err)
	return telemetry
}

func TestNewHandler_RejectsInvalidRouteTables(t *testing.T) {
	valid := route{method: http.MethodPost, path: "/extra", label: "extra", handler: http.NotFoundHandler(), errors: providerv4.NewHostErrorWriter()}
	for _, tc := range []struct {
		name   string
		mutate func(*route)
	}{
		{"empty method", func(r *route) { r.method = "" }},
		{"HEAD method", func(r *route) { r.method = http.MethodHead }},
		{"relative path", func(r *route) { r.path = "extra" }},
		{"subtree path", func(r *route) { r.path = "/extra/" }},
		{"malformed pattern", func(r *route) { r.path = "/{" }},
		{"empty label", func(r *route) { r.label = "" }},
		{"unmatched label", func(r *route) { r.label = routeUnmatched }},
		{"nil handler", func(r *route) { r.handler = nil }},
		{"nil writer", func(r *route) { r.errors = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := valid
			tc.mutate(&entry)
			_, err := newHandler(gatewayauth.AccessPolicy{Mode: gatewayauth.AccessModeSelfHosted}, newTestTelemetry(t), []route{entry})
			require.ErrorIs(t, err, errInvalidRoute)
		})
	}
	t.Run("duplicate method and path", func(t *testing.T) {
		duplicate := valid
		duplicate.label = "other"
		_, err := newHandler(gatewayauth.AccessPolicy{Mode: gatewayauth.AccessModeSelfHosted}, newTestTelemetry(t), []route{valid, duplicate})
		require.ErrorIs(t, err, errInvalidRoute)
	})
	t.Run("conflicting wildcard patterns", func(t *testing.T) {
		first, second := valid, valid
		first.path, second.path, second.label = "/a/{x}", "/{y}/b", "other"
		_, err := newHandler(gatewayauth.AccessPolicy{Mode: gatewayauth.AccessModeSelfHosted}, newTestTelemetry(t), []route{first, second})
		require.ErrorIs(t, err, errInvalidRoute)
	})
	t.Run("same path with another method", func(t *testing.T) {
		other := valid
		other.method = http.MethodGet
		_, err := newHandler(gatewayauth.AccessPolicy{Mode: gatewayauth.AccessModeSelfHosted}, newTestTelemetry(t), []route{valid, other})
		require.NoError(t, err)
	})
}

func TestProductionRoutes_MatchTheManifest(t *testing.T) {
	data := manifestdata.LocalManifest().ManifestData
	require.Len(t, data.Versions, 1)
	declared := map[string]struct{}{}
	for path, props := range data.Versions[0].OpenAPI.Paths {
		for method, operation := range map[string]*spec3.Operation{
			http.MethodGet:     props.Get,
			http.MethodPut:     props.Put,
			http.MethodPost:    props.Post,
			http.MethodDelete:  props.Delete,
			http.MethodOptions: props.Options,
			http.MethodHead:    props.Head,
			http.MethodPatch:   props.Patch,
			http.MethodTrace:   props.Trace,
		} {
			if operation != nil {
				declared[method+" "+path] = struct{}{}
			}
		}
	}
	routed := map[string]struct{}{}
	for _, entry := range productionRoutes(http.NotFoundHandler(), http.NotFoundHandler()) {
		routed[entry.method+" /namespaces/{namespace}"+entry.path] = struct{}{}
	}
	assert.Equal(t, declared, routed)
	assert.Equal(t, map[string]struct{}{
		"GET /namespaces/{namespace}/config":          {},
		"POST /namespaces/{namespace}/language-model": {},
	}, routed)
}
