package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/grafana/grafana-app-sdk/resource"

	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/service"
)

const (
	catalogPlural    = "modelcatalogs"
	catalogKind      = "ModelCatalog"
	catalogNameValue = "name"
	namespaceValue   = "namespace"
)

// catalogIdentity is the part of a parent object that names it.
type catalogIdentity struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"metadata"`
}

// parentCatalog serves a catalog route only when the platform's route parent
// is the `ModelCatalog` the route names. It stores that catalog, with its
// decrypted values, for the selector and the lister. A missing or mismatched
// parent is a platform defect, written as the internal host error before the
// body is read. A parent whose spec does not decode is stored without spec
// or keys, so its catalog fails its checks as a catalog configuration error.
func parentCatalog(errors host.ErrorWriter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		name := request.PathValue(catalogNameValue)
		info := resource.RouteRequestInfoFrom(request.Context())
		if info == nil || len(info.Parent) == 0 || info.Group != v1alpha1.APIGroup || info.Version != v1alpha1.APIVersion || info.Plural != catalogPlural || info.Name != name || info.Namespace != request.PathValue(namespaceValue) {
			errors.WriteHostError(w, host.ErrorInternal)
			return
		}
		var identity catalogIdentity
		if json.Unmarshal(info.Parent, &identity) != nil || identity.APIVersion != v1alpha1.GroupVersion.Identifier() || identity.Kind != catalogKind || identity.Metadata.Namespace != info.Namespace || identity.Metadata.Name != name {
			errors.WriteHostError(w, host.ErrorInternal)
			return
		}
		stack := service.StackCatalog{Name: name}
		var parent v1alpha1.ModelCatalog
		if json.Unmarshal(info.Parent, &parent) == nil {
			stack.Spec, stack.Keys = parent.Spec, info.DecryptedSecureValues
		}
		next.ServeHTTP(w, request.WithContext(service.WithStackCatalog(request.Context(), stack)))
	})
}
