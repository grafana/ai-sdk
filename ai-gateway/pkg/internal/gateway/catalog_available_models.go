package gateway

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/service"
)

const maxCatalogModelsQueryBytes = 256

func catalogAvailableModels(listings *service.CatalogHealthChecker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider, valid := catalogProviderSelector(r.URL.RawQuery)
		if !valid || r.ContentLength > 0 || hasCatalogActionBody(r.Body) {
			writeCatalogAction(w, http.StatusBadRequest, catalogActionError{Message: "available models requires one provider name and no body"})
			return
		}
		stack, ok := service.StackCatalogFromContext(r.Context())
		if !ok {
			catalogAvailableModelsErrors{}.WriteHostError(w, host.ErrorInternal)
			return
		}
		result, err := listings.AvailableModels(r.Context(), r.PathValue(namespaceValue), stack, provider)
		switch {
		case errors.Is(err, execution.ErrCatalogConfiguration):
			catalogAvailableModelsErrors{}.WriteHostError(w, host.ErrorCatalogConfiguration)
		case errors.Is(err, service.ErrCatalogProviderNotFound):
			writeCatalogAction(w, http.StatusNotFound, catalogActionError{Message: "provider not found in stored catalog"})
		case errors.Is(err, service.ErrCatalogHealthBusy):
			writeCatalogAction(w, http.StatusTooManyRequests, catalogActionError{Message: "a catalog listing is already running or the Gateway is busy"})
		case err != nil:
			catalogAvailableModelsErrors{}.WriteHostError(w, host.ErrorInternal)
		default:
			writeCatalogAction(w, http.StatusOK, result)
		}
	})
}

func catalogProviderSelector(raw string) (string, bool) {
	if len(raw) > maxCatalogModelsQueryBytes {
		return "", false
	}
	key, encoded, ok := strings.Cut(raw, "=")
	if !ok || key != "provider" {
		return "", false
	}
	name, err := url.QueryUnescape(encoded)
	return name, err == nil && len(validation.IsDNS1123Label(name)) == 0
}

type catalogAvailableModelsErrors struct{}

func (catalogAvailableModelsErrors) WriteHostError(w http.ResponseWriter, category host.ErrorCategory) {
	switch category {
	case host.ErrorPermission:
		writeCatalogAction(w, http.StatusForbidden, catalogActionError{Message: "access to available models denied"})
	case host.ErrorCatalogConfiguration:
		writeCatalogAction(w, http.StatusFailedDependency, catalogActionError{Message: "invalid stored catalog configuration"})
	default:
		writeCatalogAction(w, http.StatusInternalServerError, catalogActionError{Message: "available models could not be listed"})
	}
}
