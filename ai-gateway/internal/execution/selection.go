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
// logical identity. A selection built by [CatalogSelector] or
// [AccountSelection] carries the attribution its execution overview is
// projected from; any other selection publishes no overview.
type Selection struct {
	ID         string
	Model      provider.LanguageModel
	configured *configuredSelection
	accounts   *RequestAccounts
}

type configuredSelection struct {
	candidates []catalog.ConfiguredCandidate
}

// RequestAccounts is the native destination and the number of ordered accounts
// a request supplied for it.
type RequestAccounts struct {
	Provider string
	ModelID  string
	Count    int
}

// AccountSelection returns the selection for model, which tries the request's
// accounts in order against one native destination. Its overview names the
// call's requested model ID, has no canonical model or provider instance, and
// lists each account attempt with its native destination. An attempt for
// another destination, or more attempts than accounts, omits the overview.
func AccountSelection(id string, model provider.LanguageModel, accounts RequestAccounts) Selection {
	return Selection{ID: id, Model: model, accounts: &accounts}
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
