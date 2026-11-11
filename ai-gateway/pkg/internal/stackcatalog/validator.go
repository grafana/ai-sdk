package stackcatalog

import (
	"context"
	"fmt"
	"maps"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
)

// Validator is the ModelCatalog admission validator. It refuses invalid
// creates and updates with a 400 error whose message never contains a
// secure value.
type Validator struct {
	// Policy is the egress policy custom endpoints are checked against.
	Policy outbound.EgressPolicy
}

// Validate merges the request's secure values with the stored ones, as the
// platform will store them, and parses the catalog with the merged keys.
func (validator Validator) Validate(_ context.Context, request *app.AdmissionRequest) error {
	catalog, ok := request.Object.(*v1alpha1.ModelCatalog)
	if !ok {
		return apierrors.NewBadRequest("modelcatalog: the admission object is not a ModelCatalog")
	}
	var stored resource.InlineSecureValues
	if request.Action == resource.AdmissionActionUpdate {
		previous, ok := request.OldObject.(*v1alpha1.ModelCatalog)
		if !ok {
			return apierrors.NewBadRequest("modelcatalog: the stored object is not a ModelCatalog")
		}
		stored = previous.Secure
	}
	keys, err := mergeSecureKeys(stored, catalog.Secure)
	if err != nil {
		return apierrors.NewBadRequest(err.Error())
	}
	if _, err := Parse(catalog.Spec, keys, validator.Policy); err != nil {
		return apierrors.NewBadRequest(err.Error())
	}
	return nil
}

// mergeSecureKeys returns the secure keys the object holds once the platform
// applies incoming to stored: stored keys are kept unless removed, created
// keys are added, and a name is kept only when it repeats the name stored
// under the same key. Any other name would reference a secret the catalog
// does not own.
func mergeSecureKeys(stored, incoming resource.InlineSecureValues) ([]string, error) {
	keys := make(map[string]struct{}, len(stored)+len(incoming))
	for key := range stored {
		keys[key] = struct{}{}
	}
	for _, key := range slices.Sorted(maps.Keys(incoming)) {
		value := incoming[key]
		create, name := !value.Create.IsZero(), value.Name != ""
		switch {
		case value.Remove && (create || name), create && name:
			return nil, fmt.Errorf("modelcatalog: %s must set only one of create, name or remove", securePath(key))
		case value.Remove:
			delete(keys, key)
		case create:
			keys[key] = struct{}{}
		case name:
			if stored[key].Name != value.Name {
				return nil, fmt.Errorf("modelcatalog: %s must not reference a secret by name; set create with the key instead", securePath(key))
			}
			keys[key] = struct{}{}
		default:
			return nil, fmt.Errorf("modelcatalog: %s must set create, name or remove", securePath(key))
		}
	}
	return slices.Collect(maps.Keys(keys)), nil
}
