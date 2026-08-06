// Package runtime executes normalized gateway LanguageModel calls independently
// of HTTP and public protocol DTOs.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/provider"
)

// Protocol identifies the public protocol that originated a gateway call.
type Protocol string

const (
	// ProtocolLanguageModelV4 identifies the registered LanguageModelV4 wire.
	ProtocolLanguageModelV4 Protocol = "language-model-v4"
)

// GatewayCaching identifies a gateway caching policy.
type GatewayCaching string

const (
	// GatewayCachingAuto enables automatic gateway caching.
	GatewayCachingAuto GatewayCaching = "auto"
)

// GatewayCapability identifies a required gateway routing capability.
type GatewayCapability string

const (
	// GatewayCapabilityImplicitCaching requires implicit caching support.
	GatewayCapabilityImplicitCaching GatewayCapability = "implicit-caching"
)

// GatewayServiceTier identifies requested gateway service priority.
type GatewayServiceTier string

const (
	// GatewayServiceTierFlex requests the flexible service tier.
	GatewayServiceTierFlex GatewayServiceTier = "flex"
	// GatewayServiceTierPriority requests the priority service tier.
	GatewayServiceTierPriority GatewayServiceTier = "priority"
)

// GatewaySort identifies the gateway provider sort criterion.
type GatewaySort string

const (
	// GatewaySortCost prioritizes lower provider cost.
	GatewaySortCost GatewaySort = "cost"
	// GatewaySortTPS prioritizes output-token throughput.
	GatewaySortTPS GatewaySort = "tps"
	// GatewaySortTTFT prioritizes time to first token.
	GatewaySortTTFT GatewaySort = "ttft"
)

// GatewayProviderTimeouts contains per-provider timeout controls in
// milliseconds.
type GatewayProviderTimeouts struct {
	// BYOK maps provider IDs to invocation timeouts in milliseconds for
	// caller-supplied credentials.
	BYOK map[string]float64
}

// GatewayOptions contains the registered @ai-sdk/gateway@4.0.33 routing and
// attribution controls. Extensions preserves service-owned additions as opaque
// valid JSON.
type GatewayOptions struct {
	// BYOK holds private provider-keyed credential objects for gateway routing.
	BYOK map[string][]map[string]json.RawMessage
	// Caching requests the gateway caching policy.
	Caching *GatewayCaching
	// DisallowPromptTraining requests providers that do not train on prompts.
	DisallowPromptTraining *bool
	// Has lists capabilities required from a selected provider.
	Has []GatewayCapability
	// Models lists public model IDs to use as ordered fallback candidates.
	Models []string
	// Only restricts selection to the listed provider IDs.
	Only []string
	// Order gives the preferred provider-ID selection order.
	Order []string
	// ProviderTimeouts configures per-provider routing timeouts.
	ProviderTimeouts *GatewayProviderTimeouts
	// QuotaEntityID is private attribution control for gateway quota accounting.
	QuotaEntityID *string
	// ServiceTier requests a gateway service priority.
	ServiceTier *GatewayServiceTier
	// Sort requests the provider-ranking criterion.
	Sort *GatewaySort
	// Tags are private gateway attribution labels and are not response metadata.
	Tags []string
	// User is a private gateway attribution value and is not trusted host identity.
	User *string
	// ZeroDataRetention requests providers with zero-data-retention handling.
	ZeroDataRetention *bool
	// Extensions retains unknown service-owned controls as opaque valid JSON;
	// values are for policy and resolution, not provider forwarding.
	Extensions map[string]json.RawMessage
}

// Empty reports whether no gateway control was supplied.
func (options GatewayOptions) Empty() bool {
	return len(options.BYOK) == 0 &&
		options.Caching == nil &&
		options.DisallowPromptTraining == nil &&
		len(options.Has) == 0 &&
		len(options.Models) == 0 &&
		len(options.Only) == 0 &&
		len(options.Order) == 0 &&
		options.ProviderTimeouts == nil &&
		options.QuotaEntityID == nil &&
		options.ServiceTier == nil &&
		options.Sort == nil &&
		len(options.Tags) == 0 &&
		options.User == nil &&
		options.ZeroDataRetention == nil &&
		len(options.Extensions) == 0
}

// CallMetadata contains immutable metadata supplied by the trusted host.
type CallMetadata struct {
	// RequestID is the non-empty gateway-generated or host-supplied request ID.
	RequestID string
	// AuthenticatedAttributes contains identity attributes established only by
	// the trusted host; request-body and provider-header values are untrusted.
	AuthenticatedAttributes map[string]string
}

// PolicyMetadata contains policy-derived annotations. Values remain opaque to
// the runtime and are kept separate from host-authenticated attributes.
type PolicyMetadata map[string]json.RawMessage

// GatewayCall is the normalized input to gateway LanguageModel execution.
type GatewayCall struct {
	// Protocol identifies the public adapter that normalized the call.
	Protocol Protocol
	// RequestedModelID is the exact immutable public model ID supplied by the caller.
	RequestedModelID string
	// CallOptions contains normalized provider-bound input with gateway controls removed.
	CallOptions provider.CallOptions
	// GatewayOptions contains private gateway controls available to policy and resolution.
	GatewayOptions GatewayOptions
	// CallMetadata contains immutable identity metadata established by the trusted host.
	CallMetadata CallMetadata
	// PolicyMetadata contains policy-derived annotations kept separate from trusted identity.
	PolicyMetadata PolicyMetadata
}

// CallPolicy inspects or transforms a normalized call before model resolution.
type CallPolicy interface {
	Apply(ctx context.Context, call GatewayCall) (GatewayCall, error)
}

// CallPolicyFunc adapts a function to CallPolicy.
type CallPolicyFunc func(ctx context.Context, call GatewayCall) (GatewayCall, error)

// Apply implements CallPolicy.
func (f CallPolicyFunc) Apply(ctx context.Context, call GatewayCall) (GatewayCall, error) {
	return f(ctx, call)
}

func validateCall(call GatewayCall) error {
	if call.Protocol == "" {
		return failure.Wrap(failure.ErrInvalidCall, errors.New("gateway runtime: protocol is required"))
	}
	if call.RequestedModelID == "" {
		return failure.Wrap(failure.ErrInvalidCall, errors.New("gateway runtime: requested model ID is required"))
	}
	if call.CallMetadata.RequestID == "" {
		return failure.Wrap(failure.ErrInvalidCall, errors.New("gateway runtime: request ID is required"))
	}
	if err := validateProviderBoundOptions(call.CallOptions); err != nil {
		return failure.Wrap(failure.ErrInvalidCall, err)
	}
	return nil
}

func validateProviderBoundOptions(options provider.CallOptions) error {
	if hasGatewayProviderOption(options.ProviderOptions) {
		return errors.New("gateway runtime: gateway provider options must be extracted")
	}
	for i, message := range options.Prompt {
		if hasGatewayProviderOption(message.ProviderOptions) {
			return fmt.Errorf("gateway runtime: prompt message %d contains reserved gateway provider options", i)
		}
		for j, part := range message.Content {
			if hasGatewayProviderOption(part.ProviderOptions) {
				return fmt.Errorf("gateway runtime: prompt message %d content part %d contains reserved gateway provider options", i, j)
			}
			if part.Output == nil {
				continue
			}
			if hasGatewayProviderOption(part.Output.ProviderOptions) {
				return fmt.Errorf("gateway runtime: prompt message %d content part %d tool output contains reserved gateway provider options", i, j)
			}
			for k, content := range part.Output.Content {
				if hasGatewayProviderOption(content.ProviderOptions) {
					return fmt.Errorf("gateway runtime: prompt message %d content part %d tool output content %d contains reserved gateway provider options", i, j, k)
				}
			}
		}
	}
	for i, tool := range options.Tools {
		if hasGatewayProviderOption(tool.ProviderOptions) {
			return fmt.Errorf("gateway runtime: tool %d contains reserved gateway provider options", i)
		}
	}
	return nil
}

func hasGatewayProviderOption(options provider.ProviderOptions) bool {
	_, exists := options["gateway"]
	return exists
}

func applyPolicies(ctx context.Context, call GatewayCall, policies []CallPolicy) (GatewayCall, error) {
	current := cloneGatewayCall(call)
	trusted := cloneCallMetadata(current.CallMetadata)
	protocol := current.Protocol
	requestedModelID := current.RequestedModelID

	for _, policy := range policies {
		if isNilInterface(policy) {
			return GatewayCall{}, failure.Wrap(failure.ErrInternal, errors.New("gateway runtime: nil call policy"))
		}
		next, err := policy.Apply(ctx, cloneGatewayCall(current))
		if err != nil {
			return GatewayCall{}, err
		}
		if next.Protocol != protocol || next.RequestedModelID != requestedModelID {
			return GatewayCall{}, failure.Wrap(failure.ErrInternal, errors.New("gateway runtime: call policy changed immutable call identity"))
		}
		if next.CallMetadata.RequestID != trusted.RequestID || !reflect.DeepEqual(next.CallMetadata.AuthenticatedAttributes, trusted.AuthenticatedAttributes) {
			return GatewayCall{}, failure.Wrap(failure.ErrInternal, errors.New("gateway runtime: call policy changed trusted metadata"))
		}
		next.CallMetadata = cloneCallMetadata(trusted)
		if err := validateCall(next); err != nil {
			return GatewayCall{}, err
		}
		current = cloneGatewayCall(next)
	}
	return current, nil
}

func cloneGatewayCall(call GatewayCall) GatewayCall {
	call.CallOptions = cloneCallOptions(call.CallOptions)
	call.GatewayOptions = cloneGatewayOptions(call.GatewayOptions)
	call.CallMetadata = cloneCallMetadata(call.CallMetadata)
	call.PolicyMetadata = cloneRawMap(call.PolicyMetadata)
	return call
}

func cloneCallMetadata(metadata CallMetadata) CallMetadata {
	metadata.AuthenticatedAttributes = cloneStringMap(metadata.AuthenticatedAttributes)
	return metadata
}

func cloneGatewayOptions(options GatewayOptions) GatewayOptions {
	cloned := options
	cloned.Caching = clonePointer(options.Caching)
	cloned.DisallowPromptTraining = clonePointer(options.DisallowPromptTraining)
	cloned.QuotaEntityID = clonePointer(options.QuotaEntityID)
	cloned.ServiceTier = clonePointer(options.ServiceTier)
	cloned.Sort = clonePointer(options.Sort)
	cloned.User = clonePointer(options.User)
	cloned.ZeroDataRetention = clonePointer(options.ZeroDataRetention)
	if options.BYOK != nil {
		cloned.BYOK = make(map[string][]map[string]json.RawMessage, len(options.BYOK))
		for providerID, credentials := range options.BYOK {
			copied := make([]map[string]json.RawMessage, len(credentials))
			for i, credential := range credentials {
				copied[i] = cloneRawMap(credential)
			}
			cloned.BYOK[providerID] = copied
		}
	}
	cloned.Has = append([]GatewayCapability(nil), options.Has...)
	cloned.Models = append([]string(nil), options.Models...)
	cloned.Only = append([]string(nil), options.Only...)
	cloned.Order = append([]string(nil), options.Order...)
	cloned.Tags = append([]string(nil), options.Tags...)
	if options.ProviderTimeouts != nil {
		cloned.ProviderTimeouts = &GatewayProviderTimeouts{BYOK: cloneFloatMap(options.ProviderTimeouts.BYOK)}
	}
	cloned.Extensions = cloneRawMap(options.Extensions)
	return cloned
}

func cloneCallOptions(options provider.CallOptions) provider.CallOptions {
	cloned := options
	cloned.MaxOutputTokens = clonePointer(options.MaxOutputTokens)
	cloned.Temperature = clonePointer(options.Temperature)
	cloned.TopP = clonePointer(options.TopP)
	cloned.TopK = clonePointer(options.TopK)
	cloned.PresencePenalty = clonePointer(options.PresencePenalty)
	cloned.FrequencyPenalty = clonePointer(options.FrequencyPenalty)
	cloned.Seed = clonePointer(options.Seed)
	cloned.Reasoning = clonePointer(options.Reasoning)
	cloned.Prompt = make([]provider.Message, len(options.Prompt))
	for i, message := range options.Prompt {
		cloned.Prompt[i] = message
		cloned.Prompt[i].Content = make([]provider.ContentPart, len(message.Content))
		for j, part := range message.Content {
			cloned.Prompt[i].Content[j] = cloneContentPart(part)
		}
	}
	if options.Tools != nil {
		cloned.Tools = make([]provider.Tool, len(options.Tools))
		for i, tool := range options.Tools {
			cloned.Tools[i] = tool
			cloned.Tools[i].InputSchema = cloneRaw(tool.InputSchema)
			cloned.Tools[i].Strict = clonePointer(tool.Strict)
			if tool.InputExamples != nil {
				cloned.Tools[i].InputExamples = make([]provider.InputExample, len(tool.InputExamples))
				for j, example := range tool.InputExamples {
					cloned.Tools[i].InputExamples[j] = provider.InputExample{Input: cloneRaw(example.Input)}
				}
			}
			cloned.Tools[i].Args = cloneRawMap(tool.Args)
			cloned.Tools[i].ProviderOptions = cloneProviderOptions(tool.ProviderOptions)
		}
	}
	if options.ToolChoice != nil {
		value := *options.ToolChoice
		cloned.ToolChoice = &value
	}
	if options.ResponseFormat != nil {
		value := *options.ResponseFormat
		value.Schema = cloneRaw(value.Schema)
		cloned.ResponseFormat = &value
	}
	cloned.StopSequences = append([]string(nil), options.StopSequences...)
	cloned.Headers = cloneStringMap(options.Headers)
	cloned.ProviderOptions = cloneProviderOptions(options.ProviderOptions)
	return cloned
}

func cloneContentPart(part provider.ContentPart) provider.ContentPart {
	cloned := part
	cloned.Input = cloneRaw(part.Input)
	cloned.ProviderOptions = cloneProviderOptions(part.ProviderOptions)
	if part.Data != nil {
		value := *part.Data
		value.Bytes = append([]byte(nil), part.Data.Bytes...)
		value.Reference = cloneRaw(part.Data.Reference)
		cloned.Data = &value
	}
	if part.Output != nil {
		value := *part.Output
		value.JSON = cloneRaw(part.Output.JSON)
		value.ProviderOptions = cloneProviderOptions(part.Output.ProviderOptions)
		value.Content = make([]provider.ToolResultContentValue, len(part.Output.Content))
		for i, content := range part.Output.Content {
			value.Content[i] = content
			value.Content[i].ProviderOptions = cloneProviderOptions(content.ProviderOptions)
			if content.Data != nil {
				data := *content.Data
				data.Bytes = append([]byte(nil), content.Data.Bytes...)
				data.Reference = cloneRaw(content.Data.Reference)
				value.Content[i].Data = &data
			}
		}
		cloned.Output = &value
	}
	if part.Approved != nil {
		value := *part.Approved
		cloned.Approved = &value
	}
	return cloned
}

func cloneProviderOptions(options provider.ProviderOptions) provider.ProviderOptions {
	if options == nil {
		return nil
	}
	cloned := make(provider.ProviderOptions, len(options))
	for key, option := range options {
		if raw, ok := option.(provider.RawProviderOption); ok {
			raw.Raw = cloneRaw(raw.Raw)
			cloned[key] = raw
			continue
		}
		cloned[key] = option
	}
	return cloned
}

func cloneRawMap[M ~map[string]json.RawMessage](values M) M {
	if values == nil {
		return nil
	}
	cloned := make(M, len(values))
	for key, value := range values {
		cloned[key] = cloneRaw(value)
	}
	return cloned
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneFloatMap(values map[string]float64) map[string]float64 {
	if values == nil {
		return nil
	}
	cloned := make(map[string]float64, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func unsupportedGatewayOptionsError() error {
	return failure.Wrap(failure.ErrInvalidCall, fmt.Errorf("gateway runtime: configured catalog resolver does not support gateway routing controls"))
}
