// Package discovery serves model discovery for an adapter: the host decides
// access and lists the catalog, and the adapter projects the listed models.
package discovery

import (
	"context"
	"errors"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
)

var (
	errListingPanicked    = errors.New("gateway discovery: listing models panicked")
	errProjectionPanicked = errors.New("gateway discovery: projection panicked")
)

type handler struct {
	configured catalog.ModelLister
	stack      catalog.ModelLister
	projection host.DiscoveryProjection
	errors     host.ErrorWriter
}

// New constructs a discovery handler. Configured callers get the projection of
// the configured catalog listed under the request context, and catalog
// callers that of the stack catalog their route serves. BYOK callers get the
// BYOK discovery error without a listing, and other callers get the
// permission error. A stack catalog that fails its checks is written as the
// catalog configuration error, and other listing and projection failures as
// internal errors.
func New(configured, stack catalog.ModelLister, projection host.DiscoveryProjection, errorWriter host.ErrorWriter) http.Handler {
	return &handler{configured: configured, stack: stack, projection: projection, errors: errorWriter}
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	caller, _ := gatewayauth.CallerFromContext(request.Context())
	var lister catalog.ModelLister
	switch caller.AccountAccess() {
	case gatewayauth.ConfiguredAccounts:
		lister = handler.configured
	case gatewayauth.CatalogAccounts:
		lister = handler.stack
	case gatewayauth.RequestBYOK:
		handler.errors.WriteHostError(w, host.ErrorBYOKDiscovery)
		return
	default:
		handler.errors.WriteHostError(w, host.ErrorPermission)
		return
	}
	models, err := safeList(request.Context(), lister)
	if errors.Is(err, execution.ErrCatalogConfiguration) {
		handler.errors.WriteHostError(w, host.ErrorCatalogConfiguration)
		return
	}
	if err != nil {
		handler.errors.WriteHostError(w, host.ErrorInternal)
		return
	}
	document, err := handler.safeProject(models)
	if err != nil {
		handler.errors.WriteHostError(w, host.ErrorInternal)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
}

func safeList(ctx context.Context, lister catalog.ModelLister) (models []catalog.ModelInfo, err error) {
	defer func() {
		if recover() != nil {
			models, err = nil, errListingPanicked
		}
	}()
	return lister.ListModels(ctx)
}

func (handler *handler) safeProject(models []catalog.ModelInfo) (document []byte, err error) {
	defer func() {
		if recover() != nil {
			document, err = nil, errProjectionPanicked
		}
	}()
	return handler.projection.ProjectDiscovery(models)
}
