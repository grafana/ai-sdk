// Package discovery serves model discovery for an adapter: the host decides
// access and lists the catalog, and the adapter projects the listed models.
package discovery

import (
	"context"
	"errors"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
)

var (
	errListingPanicked    = errors.New("gateway discovery: listing models panicked")
	errProjectionPanicked = errors.New("gateway discovery: projection panicked")
)

type handler struct {
	lister     catalog.ModelLister
	projection host.DiscoveryProjection
	errors     host.ErrorWriter
	resource   host.DiscoveryResourceProjection
}

// New constructs a discovery handler. Configured callers get the projection of
// the catalog listed under the request context, BYOK callers get the BYOK
// discovery error without a listing, and other callers get the permission
// error. Listing and projection failures are written as internal errors.
func New(lister catalog.ModelLister, projection host.DiscoveryProjection, errorWriter host.ErrorWriter) http.Handler {
	return &handler{lister: lister, projection: projection, errors: errorWriter}
}

// NewResource retrieves a public model from the same caller-visible catalog and
// access policy as discovery. The route supplies the decoded model path value.
func NewResource(lister catalog.ModelLister, projection host.DiscoveryResourceProjection, errorWriter host.ErrorWriter) http.Handler {
	return &handler{lister: lister, resource: projection, errors: errorWriter}
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	caller, _ := gatewayauth.CallerFromContext(request.Context())
	switch caller.AccountAccess() {
	case gatewayauth.ConfiguredAccounts:
	case gatewayauth.RequestBYOK:
		handler.errors.WriteHostError(w, host.ErrorBYOKDiscovery)
		return
	default:
		handler.errors.WriteHostError(w, host.ErrorPermission)
		return
	}
	models, err := handler.safeList(request.Context())
	if err != nil {
		handler.errors.WriteHostError(w, host.ErrorInternal)
		return
	}
	var document []byte
	if handler.resource != nil {
		var found bool
		document, found, err = handler.safeResource(models, request.PathValue("model"))
		if err == nil && !found {
			handler.errors.WriteHostError(w, host.ErrorModelNotFound)
			return
		}
	} else {
		document, err = handler.safeProject(models)
	}
	if err != nil {
		handler.errors.WriteHostError(w, host.ErrorInternal)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
}

func (handler *handler) safeList(ctx context.Context) (models []catalog.ModelInfo, err error) {
	defer func() {
		if recover() != nil {
			models, err = nil, errListingPanicked
		}
	}()
	return handler.lister.ListModels(ctx)
}

func (handler *handler) safeProject(models []catalog.ModelInfo) (document []byte, err error) {
	defer func() {
		if recover() != nil {
			document, err = nil, errProjectionPanicked
		}
	}()
	return handler.projection.ProjectDiscovery(models)
}

func (handler *handler) safeResource(models []catalog.ModelInfo, id string) (document []byte, found bool, err error) {
	defer func() {
		if recover() != nil {
			document = nil
			found = false
			err = errProjectionPanicked
		}
	}()
	return handler.resource.ProjectDiscoveryResource(models, id)
}
