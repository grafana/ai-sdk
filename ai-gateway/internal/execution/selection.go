package execution

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

// Selection is the executable model a selector chose for one request, with its
// logical identity.
type Selection struct {
	ID         string
	Model      provider.LanguageModel
	configured *configuredSelection
}

type configuredSelection struct {
	candidates []catalog.ConfiguredCandidate
}

// ConfiguredCandidates reports the configured chain behind a catalog
// selection, and false for a selection that did not come from the catalog.
func (s Selection) ConfiguredCandidates() ([]catalog.ConfiguredCandidate, bool) {
	if s.configured == nil {
		return nil, false
	}
	return s.configured.candidates, true
}

// RequestSelector chooses the model for a request from its model ID, its mapped
// options and the raw call-level gateway controls, which are nil when absent.
type RequestSelector func(ctx context.Context, modelID string, options provider.CallOptions, gateway json.RawMessage) (Selection, error)

var (
	// ErrReservedProviderOptions reports gateway controls a selector does not accept.
	ErrReservedProviderOptions = errors.New("execution: reserved provider options")
	// ErrInvalidBYOK reports invalid or unsupported BYOK credentials.
	ErrInvalidBYOK = errors.New("execution: invalid BYOK credentials")
	// ErrInvalidBYOKSelector reports a model ID that BYOK cannot select.
	ErrInvalidBYOKSelector = errors.New("execution: invalid BYOK model selector")
	// ErrUnsupportedGatewayControl reports a gateway control other than byok.
	ErrUnsupportedGatewayControl = errors.New("execution: unsupported gateway control")
	// ErrAccountAccess reports a caller without access to the selected accounts.
	ErrAccountAccess = errors.New("execution: account access denied")
)

// CatalogSelector selects configured models from resolver. It refuses gateway
// controls, and returns nil for a nil resolver.
func CatalogSelector(resolver catalog.ModelResolver) RequestSelector {
	if isNilInterface(resolver) {
		return nil
	}
	return func(ctx context.Context, modelID string, _ provider.CallOptions, gateway json.RawMessage) (Selection, error) {
		if gateway != nil {
			return Selection{}, ErrReservedProviderOptions
		}
		resolved, err := resolver.ResolveModel(ctx, modelID)
		if err != nil {
			return Selection{}, err
		}
		return Selection{
			ID:    resolved.ID,
			Model: resolved.Model,
			configured: &configuredSelection{
				candidates: slices.Clone(resolved.Candidates),
			},
		}, nil
	}
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
