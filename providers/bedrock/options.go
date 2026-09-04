package bedrock

import (
	"encoding/json"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// Option configures a Bedrock model instance.
type Option func(*model)

// WithRegion sets the AWS region used to construct the default Bedrock
// endpoint URL and to scope SigV4 signatures.
func WithRegion(region string) Option {
	return func(m *model) { m.region = region }
}

// WithBaseURL overrides the Bedrock endpoint base URL. Use for VPC endpoints,
// test servers, alternate AWS partitions, or Bedrock Mantle
// (`https://bedrock-mantle.<region>.api.aws`). When unset, the provider builds
// the default URL `https://bedrock-runtime.<region>.amazonaws.com`.
//
// When the base URL host is a Bedrock Mantle endpoint
// (`bedrock-mantle.<region>.api.aws`), SigV4 signing automatically scopes the
// signature to the "bedrock-mantle" service. Use [WithSigningService] to
// override that inference (for example, behind a proxy base URL).
func WithBaseURL(baseURL string) Option {
	return func(m *model) { m.baseURL = baseURL }
}

// WithSigningService overrides the AWS service name used in the SigV4
// credential scope. Leave unset to infer the service from the endpoint host:
// "bedrock-mantle" for Bedrock Mantle endpoints and "bedrock" otherwise.
//
// Set this explicitly when the endpoint host does not encode the service --
// for example, when reaching Bedrock Mantle through a proxy or VPC endpoint
// whose host is not `bedrock-mantle.<region>.api.aws`. Has no effect when a
// bearer token is configured.
func WithSigningService(service string) Option {
	return func(m *model) { m.signingService = service }
}

// WithCredentials supplies an aws.CredentialsProvider used to sign requests
// with SigV4. When unset and no bearer token is configured, the provider
// loads the default AWS credential chain at first call.
func WithCredentials(cp aws.CredentialsProvider) Option {
	return func(m *model) { m.credentials = cp }
}

// WithBearerToken authenticates with `Authorization: Bearer <token>` and
// skips SigV4. Takes precedence over any credentials provider. When unset,
// the constructor falls back to the AWS_BEARER_TOKEN_BEDROCK environment
// variable.
func WithBearerToken(token string) Option {
	return func(m *model) { m.bearerToken = token }
}

// WithHTTPClient sets the HTTP client used for outbound requests. Defaults
// to http.DefaultClient. Useful for tests, custom timeouts, or proxies.
func WithHTTPClient(client *http.Client) Option {
	return func(m *model) { m.httpClient = client }
}

// WithHeaders attaches static headers to every outbound request. Headers are
// merged before SigV4 signing so the signature covers them.
func WithHeaders(headers map[string]string) Option {
	return func(m *model) {
		if m.headers == nil {
			m.headers = make(map[string]string, len(headers))
		}
		for k, v := range headers {
			m.headers[k] = v
		}
	}
}

// WithGenerateID overrides the random tool-use ID generator used when the
// server omits IDs (rare). Intended for tests.
func WithGenerateID(gen func() string) Option {
	return func(m *model) { m.generateID = gen }
}

// BedrockOptions carries Bedrock-specific request options from
// CallOptions.ProviderOptions["amazonBedrock"]. Upstream also accepts the
// legacy key "bedrock"; both are honored at request build time.
type BedrockOptions struct {
	passthrough map[string]json.RawMessage

	// ReasoningConfig configures Anthropic-on-Bedrock extended thinking. Only
	// applied for Anthropic models; emits a warning otherwise.
	ReasoningConfig *ReasoningConfig `json:"reasoningConfig,omitempty"`
	// AnthropicBeta enumerates Anthropic beta flags to forward via
	// additionalModelRequestFields.anthropic_beta. Only meaningful for
	// Anthropic models on Bedrock.
	AnthropicBeta []string `json:"anthropicBeta,omitempty"`
	// CachePoint configures Bedrock prompt caching for the system messages or
	// the message it is attached to.
	CachePoint *CachePoint `json:"cachePoint,omitempty"`
	// ServiceTier optionally pins the Bedrock service tier (e.g., "priority").
	ServiceTier string `json:"serviceTier,omitempty"`
	// AdditionalModelRequestFields lets callers pass arbitrary key/value pairs
	// through the Converse additionalModelRequestFields pass-through.
	AdditionalModelRequestFields map[string]any `json:"additionalModelRequestFields,omitempty"`
}

func (o BedrockOptions) topLevelPassthrough() map[string]json.RawMessage {
	fields := make(map[string]json.RawMessage, len(o.passthrough)+2)
	for key, value := range o.passthrough {
		fields[key] = value
	}
	if _, ok := fields["anthropicBeta"]; !ok && o.AnthropicBeta != nil {
		fields["anthropicBeta"] = jsonRawOrZero(o.AnthropicBeta)
	}
	if _, ok := fields["cachePoint"]; !ok && o.CachePoint != nil {
		fields["cachePoint"] = jsonRawOrZero(o.CachePoint)
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func (o *BedrockOptions) UnmarshalJSON(data []byte) error {
	type bedrockOptions BedrockOptions

	var decoded bedrockOptions
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{
		"reasoningConfig",
		"additionalModelRequestFields",
		"serviceTier",
	} {
		delete(fields, key)
	}

	*o = BedrockOptions(decoded)
	if len(fields) > 0 {
		o.passthrough = fields
	}
	return nil
}

// ProviderKey returns the provider option namespace. Matches upstream
// `providerOptions.amazonBedrock`.
func (BedrockOptions) ProviderKey() string { return "amazonBedrock" }

// ReasoningConfig configures Anthropic-on-Bedrock extended thinking and
// reasoning effort. Mirrors upstream amazonBedrock reasoningConfig.
type ReasoningConfig struct {
	// Type is one of "enabled", "adaptive", or empty when only
	// MaxReasoningEffort is set.
	Type string `json:"type,omitempty"`
	// BudgetTokens is the thinking budget for Type="enabled". Required when
	// Type is "enabled".
	BudgetTokens *int `json:"budgetTokens,omitempty"`
	// Display controls adaptive thinking display ("auto" or "always"). Only
	// meaningful when Type is "adaptive".
	Display string `json:"display,omitempty"`
	// MaxReasoningEffort routes to Anthropic `output_config.effort`, OpenAI
	// `reasoning_effort`, or Nova `reasoningConfig.maxReasoningEffort`
	// depending on the model.
	MaxReasoningEffort string `json:"maxReasoningEffort,omitempty"`
}

// CachePoint configures Bedrock prompt caching.
type CachePoint struct {
	// Type currently has a single accepted value, "default".
	Type string `json:"type,omitempty"`
	// TTL is one of "5m" or "1h". 5-minute TTL is the default and supported
	// by all caching-enabled models; 1-hour TTL is supported by select models
	// (Claude Opus/Haiku/Sonnet 4.5).
	TTL string `json:"ttl,omitempty"`
}

// GuardContentQualifier identifies a Bedrock guardrail text qualifier.
type GuardContentQualifier string

const (
	GuardContentGroundingSource GuardContentQualifier = "grounding_source"
	GuardContentQuery           GuardContentQualifier = "query"
	GuardContentGuardContent    GuardContentQualifier = "guard_content"
)

// TextPartOptions carries per-text-part Bedrock guardrail configuration.
type TextPartOptions struct {
	GuardContent           bool                    `json:"guardContent,omitempty"`
	GuardContentQualifiers []GuardContentQualifier `json:"guardContentQualifiers,omitempty"`
}

// ProviderKey returns the per-part Bedrock option namespace.
func (TextPartOptions) ProviderKey() string { return "amazonBedrock" }

// ImagePartOptions carries per-image-part Bedrock guardrail configuration.
type ImagePartOptions struct {
	GuardContent bool `json:"guardContent,omitempty"`
}

// ProviderKey returns the per-part Bedrock option namespace.
func (ImagePartOptions) ProviderKey() string { return "amazonBedrock" }

// FilePartOptions carries per-file-part Bedrock configuration from
// `ContentPart.ProviderOptions["amazonBedrock"]`. Mirrors upstream
// amazonBedrockFilePartProviderOptions, which only exposes citations at the
// part level (prompt-cache breakpoints are message-level only).
type FilePartOptions struct {
	// Citations enables Bedrock document citations on the part it's attached
	// to.
	Citations *FilePartCitations `json:"citations,omitempty"`
}

// FilePartCitations toggles Bedrock document citations.
type FilePartCitations struct {
	Enabled bool `json:"enabled"`
}

// ProviderKey returns the per-part Bedrock option namespace.
func (FilePartOptions) ProviderKey() string { return "amazonBedrock" }

// ReasoningMetadata carries Bedrock reasoning provider metadata round-trips.
// Mirrors upstream amazonBedrockReasoningMetadata.
type ReasoningMetadata struct {
	Signature       string `json:"signature,omitempty"`
	RedactedContent string `json:"redactedContent,omitempty"`
	RedactedData    string `json:"redactedData,omitempty"`
}

// ProviderKey returns the reasoning metadata namespace.
func (ReasoningMetadata) ProviderKey() string { return "amazonBedrock" }

// jsonRawOrZero returns the JSON encoding of v, or `null` on error. Used by
// constructors that need to inline a struct as JSON.
func jsonRawOrZero(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}
