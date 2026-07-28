package enrichment

import (
	"context"
	"strings"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

const (
	defaultMaxValueLength = 1024
	redactedValue         = "[REDACTED]"
)

// Cardinality describes the expected cardinality of an enrichment value.
type Cardinality string

const (
	// CardinalityLow is for small bounded enums such as environment or plan tier.
	CardinalityLow Cardinality = "low"
	// CardinalityBounded is for deployment-bounded identifiers such as tenant or region.
	CardinalityBounded Cardinality = "bounded"
	// CardinalityHigh is for request, trace, user, or session identifiers.
	CardinalityHigh Cardinality = "high"
)

// ConflictPolicy controls how enrichment handles caller-provided values.
type ConflictPolicy string

const (
	// ConflictCallerWins preserves existing caller values on conflicts.
	ConflictCallerWins ConflictPolicy = "caller_wins"
	// ConflictEnrichmentWins overwrites existing caller values on conflicts when safe.
	ConflictEnrichmentWins ConflictPolicy = "enrichment_wins"
	// ConflictError returns an error on conflicts.
	ConflictError ConflictPolicy = "error"
)

// Value is one string enrichment value collected from explicit request context.
type Value struct {
	Key         string
	Value       string
	Sensitive   bool
	Cardinality Cardinality
}

// ValueOption configures metadata for a value created by WithValue.
type ValueOption func(*Value)

// Sensitive marks a value as sensitive.
func Sensitive() ValueOption { return func(v *Value) { v.Sensitive = true } }

// WithCardinality sets a value's cardinality metadata.
func WithCardinality(cardinality Cardinality) ValueOption {
	return func(v *Value) { v.Cardinality = cardinality }
}

// CallInput describes the provider call being enriched.
type CallInput struct {
	Type   middleware.CallType
	Params provider.CallOptions
	Model  provider.LanguageModel
}

// DynamicValuesFunc returns request-derived enrichment values for a provider call.
type DynamicValuesFunc func(ctx context.Context, input CallInput) ([]Value, error)

// FilterOptions configures default-deny filtering and value normalization.
type FilterOptions struct {
	Include             []string
	Exclude             []string
	RedactSensitive     bool
	DropHighCardinality bool
	MaxValueLength      int
}

// HeaderOptions configures enrichment into provider call headers.
type HeaderOptions struct {
	Map                 map[string]string
	Prefix              string
	Conflict            ConflictPolicy
	AdditionalProtected []string
}

// ProviderOptionsConfig configures enrichment into provider options JSON.
type ProviderOptionsConfig struct {
	ProviderKey string
	ObjectKey   string
	Map         map[string]string
	Conflict    ConflictPolicy
}

// Redactor can transform, mark sensitive, or drop enrichment values.
type Redactor interface {
	RedactValue(ctx context.Context, value Value) (Value, bool)
}

// RedactorFunc adapts a function to Redactor.
type RedactorFunc func(ctx context.Context, value Value) (Value, bool)

// RedactValue implements Redactor.
func (f RedactorFunc) RedactValue(ctx context.Context, value Value) (Value, bool) {
	return f(ctx, value)
}

// Options configures enrichment middleware.
type Options struct {
	Values          []Value
	ContextValues   bool
	DynamicValues   DynamicValuesFunc
	Headers         HeaderOptions
	ProviderOptions ProviderOptionsConfig
	Filter          FilterOptions
	Redactor        Redactor
	OnError         func(context.Context, error) error
}

type normalizedOptions struct {
	values          []Value
	contextValues   bool
	dynamicValues   DynamicValuesFunc
	headers         HeaderOptions
	providerOptions ProviderOptionsConfig
	filter          FilterOptions
	redactor        Redactor
	onError         func(context.Context, error) error
}

func normalizeOptions(opts Options) normalizedOptions {
	filter := opts.Filter
	if filter.MaxValueLength <= 0 {
		filter.MaxValueLength = defaultMaxValueLength
	}

	headers := opts.Headers
	if headers.Conflict == "" {
		headers.Conflict = ConflictCallerWins
	}

	providerOptions := opts.ProviderOptions
	if providerOptions.Conflict == "" {
		providerOptions.Conflict = ConflictCallerWins
	}

	redactor := opts.Redactor
	if redactor == nil {
		redactor = DefaultRedactor()
	}

	return normalizedOptions{
		values:          cloneValues(opts.Values),
		contextValues:   opts.ContextValues,
		dynamicValues:   opts.DynamicValues,
		headers:         headers,
		providerOptions: providerOptions,
		filter:          filter,
		redactor:        redactor,
		onError:         opts.OnError,
	}
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
}

func isSelected(key string, include, specific, exclude map[string]struct{}) bool {
	if _, ok := exclude[key]; ok {
		return false
	}
	if _, ok := include[key]; ok {
		return true
	}
	if _, ok := specific[key]; ok {
		return true
	}
	return false
}

func canonicalConflictPolicy(policy ConflictPolicy) ConflictPolicy {
	switch policy {
	case ConflictEnrichmentWins, ConflictError:
		return policy
	default:
		return ConflictCallerWins
	}
}

func cloneValues(values []Value) []Value {
	if len(values) == 0 {
		return nil
	}
	out := make([]Value, len(values))
	copy(out, values)
	return out
}

func normalizedKey(key string) string { return strings.TrimSpace(key) }
