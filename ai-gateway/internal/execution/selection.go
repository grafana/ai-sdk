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
// [ConfiguredSelection] or [AccountSelection] carries the expected attempts its
// execution overview is projected from; any other selection publishes no overview.
type Selection struct {
	ID          string
	Model       provider.LanguageModel
	attribution attribution
	expected    []ExpectedAttempt
}

type attribution uint8

const (
	attributionNone attribution = iota
	attributionConfigured
	attributionAccounts
)

// ExpectedAttempt is the destination one attempt of a selection's chain is
// expected to reach and the model whose chain the attempt belongs to.
type ExpectedAttempt struct {
	// ModelID is the requested model ID or models entry, as sent.
	ModelID string
	// CanonicalID is the model's canonical ID, empty for request accounts.
	CanonicalID string
	// Destination is the expected destination. A request-account destination
	// has only a provider and model.
	Destination catalog.ConfiguredCandidate
}

// ConfiguredSelection returns the selection for model, which tries the expected
// attempts in order. id is the requested model's canonical ID, which is the
// overview's canonical identity even when none of that model's candidates were
// composed. The overview attributes each attempt to the configured destination
// expected at its position; without expected attempts, attempts keep their
// decision identity.
func ConfiguredSelection(id string, model provider.LanguageModel, expected []ExpectedAttempt) Selection {
	return Selection{ID: id, Model: model, attribution: attributionConfigured, expected: slices.Clone(expected)}
}

// AccountSelection returns the selection for model, which tries the request's
// accounts in the order of expected. Its overview names the call's requested
// model ID, has no canonical model or provider instance, and lists each account
// attempt with its native destination. An attempt for another destination than
// the one expected at its position, or more attempts than expected, omits the
// overview.
func AccountSelection(id string, model provider.LanguageModel, expected []ExpectedAttempt) Selection {
	return Selection{ID: id, Model: model, attribution: attributionAccounts, expected: slices.Clone(expected)}
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
		return Selection{ID: resolved.ID, Model: resolved.Model, attribution: attributionConfigured, expected: configuredExpected(modelID, resolved.ID, resolved.Candidates)}, nil
	}
}

// configuredExpected returns the expected attempts of one configured model's
// candidates, in order.
func configuredExpected(modelID, canonical string, candidates []catalog.ConfiguredCandidate) []ExpectedAttempt {
	expected := make([]ExpectedAttempt, len(candidates))
	for i, candidate := range candidates {
		expected[i] = ExpectedAttempt{ModelID: modelID, CanonicalID: canonical, Destination: candidate}
	}
	return expected
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
