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
// logical identity. A selection built by [CatalogSelector],
// [ConfiguredSelection] or [AccountSelection] carries the ordered model groups
// its execution overview is projected from; any other selection publishes no
// overview.
type Selection struct {
	ID     string
	Model  provider.LanguageModel
	groups []modelGroup
}

// modelGroup is one model of a selection's chain and the destinations its
// attempts are expected to reach, in attempt order. A configured group has
// candidates, or none when only decision identity is known; a request-account
// group has accounts.
type modelGroup struct {
	requested  string
	canonical  string
	candidates []catalog.ConfiguredCandidate
	accounts   *RequestAccounts
}

// ConfiguredModel is one catalog model of a configured selection: the ID the
// request named, its canonical ID and its destinations in attempt order.
type ConfiguredModel struct {
	RequestedID string
	CanonicalID string
	Candidates  []catalog.ConfiguredCandidate
}

// ConfiguredSelection returns the selection for model, which tries the
// candidates of models in order. id is the requested model's canonical ID,
// which is the overview's canonical identity even when none of that model's
// candidates were composed. The overview attributes each attempt to the
// configured candidate at its position.
func ConfiguredSelection(id string, model provider.LanguageModel, models []ConfiguredModel) Selection {
	selected := Selection{ID: id, Model: model, groups: make([]modelGroup, 0, len(models))}
	for _, configured := range models {
		selected.groups = append(selected.groups, modelGroup{
			requested:  configured.RequestedID,
			canonical:  configured.CanonicalID,
			candidates: slices.Clone(configured.Candidates),
		})
	}
	return selected
}

// RequestAccounts is one native destination and the number of ordered accounts
// a request supplied for it.
type RequestAccounts struct {
	Provider string
	ModelID  string
	Count    int
}

// AccountSelection returns the selection for model, which tries the request's
// accounts in order, destination by destination. Its overview names the call's
// requested model ID, has no canonical model or provider instance, and lists
// each account attempt with its native destination. An attempt for another
// destination than the one at its position, or more attempts than accounts,
// omits the overview.
func AccountSelection(id string, model provider.LanguageModel, destinations []RequestAccounts) Selection {
	selected := Selection{ID: id, Model: model, groups: make([]modelGroup, 0, len(destinations))}
	for _, accounts := range destinations {
		selected.groups = append(selected.groups, modelGroup{
			requested: accounts.Provider + "/" + accounts.ModelID,
			accounts:  &accounts,
		})
	}
	return selected
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
	// ErrUnsupportedGatewayControl reports a gateway control the caller's access
	// does not accept.
	ErrUnsupportedGatewayControl = errors.New("execution: unsupported gateway control")
	// ErrAccountAccess reports a caller without access to the selected accounts.
	ErrAccountAccess = errors.New("execution: account access denied")
	// ErrRoutingControls reports malformed models, order or only controls.
	ErrRoutingControls = errors.New("execution: invalid routing controls")
	// ErrProviderTimeouts reports an invalid providerTimeouts.byok map.
	ErrProviderTimeouts = errors.New("execution: invalid provider timeouts")
	// ErrNoRoutingCandidates reports an only filter that excludes every candidate.
	ErrNoRoutingCandidates = errors.New("execution: routing excludes every candidate")
	// ErrRoutingAttemptBound reports a composed chain above the attempt bound.
	ErrRoutingAttemptBound = errors.New("execution: routing exceeds the attempt bound")
	// ErrUnknownFallbackModel reports a models entry that is not an available model.
	ErrUnknownFallbackModel = errors.New("execution: unknown fallback model")
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
		return ConfiguredSelection(resolved.ID, resolved.Model, []ConfiguredModel{{
			RequestedID: modelID,
			CanonicalID: resolved.ID,
			Candidates:  resolved.Candidates,
		}}), nil
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
