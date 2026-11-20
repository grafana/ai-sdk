package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/service"
)

func catalogHealth(checker *service.CatalogHealthChecker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" || r.ContentLength > 0 || hasCatalogActionBody(r.Body) {
			writeCatalogAction(w, http.StatusBadRequest, catalogActionError{Message: "catalog health does not accept a body or query parameters"})
			return
		}
		stack, ok := service.StackCatalogFromContext(r.Context())
		if !ok {
			catalogHealthErrors{}.WriteHostError(w, host.ErrorInternal)
			return
		}
		result, err := checker.Check(r.Context(), r.PathValue(namespaceValue), stack)
		switch {
		case errors.Is(err, execution.ErrCatalogConfiguration):
			catalogHealthErrors{}.WriteHostError(w, host.ErrorCatalogConfiguration)
		case errors.Is(err, service.ErrCatalogHealthBusy):
			writeCatalogAction(w, http.StatusTooManyRequests, catalogActionError{Message: "a catalog health check is already running or the Gateway is busy"})
		case err != nil:
			catalogHealthErrors{}.WriteHostError(w, host.ErrorInternal)
		default:
			writeCatalogAction(w, http.StatusOK, result)
		}
	})
}

func hasCatalogActionBody(body io.Reader) bool {
	if body == nil || body == http.NoBody {
		return false
	}
	var b [1]byte
	_, err := io.ReadFull(body, b[:])
	return err != io.EOF
}

type catalogActionError struct {
	Message string `json:"message"`
}

type catalogHealthErrors struct{}

func (catalogHealthErrors) WriteHostError(w http.ResponseWriter, category host.ErrorCategory) {
	switch category {
	case host.ErrorPermission:
		writeCatalogAction(w, http.StatusForbidden, catalogActionError{Message: "access to catalog health denied"})
	case host.ErrorCatalogConfiguration:
		writeCatalogAction(w, http.StatusFailedDependency, catalogActionError{Message: "invalid stored catalog configuration"})
	default:
		writeCatalogAction(w, http.StatusInternalServerError, catalogActionError{Message: "catalog health check failed"})
	}
}

func writeCatalogAction(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
