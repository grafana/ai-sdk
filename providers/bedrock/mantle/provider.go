package mantle

import (
	"context"
	"fmt"
	"os"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/grafana/ai-sdk/provider"
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	openaibedrock "github.com/openai/openai-go/v3/bedrock"
	"github.com/openai/openai-go/v3/option"
)

const (
	responsesProviderName = "bedrock-mantle.responses"
	gpt56LunaModelID      = "openai.gpt-5.6-luna"
)

// Config configures Bedrock Mantle routing and authentication.
//
// Authentication is selected in this order: explicit bearer configuration,
// explicit AWS configuration, AWS_BEARER_TOKEN_BEDROCK, then the standard AWS
// credential chain. Explicit authentication modes are mutually exclusive.
type Config = openaibedrock.Config

// TokenProvider resolves a Bedrock bearer credential before each request
// attempt, allowing expiring credentials to be refreshed for retries.
type TokenProvider = openaibedrock.TokenProvider

// NewResponses constructs a language model for Bedrock Mantle's
// OpenAI-compatible Responses API.
//
// Generic client options such as option.WithHTTPClient, option.WithMaxRetries,
// and option.WithHeader may be supplied in clientOpts. Authentication and the
// base URL must be configured through Config so the official Bedrock client can
// validate and finalize every request safely. When Config.BaseURL is empty,
// generic Mantle models use /v1 and GPT-5.6 Luna uses its model-specific
// /openai/v1 route.
func NewResponses(ctx context.Context, modelID string, cfg Config, clientOpts ...option.RequestOption) (provider.LanguageModel, error) {
	if err := applyDefaultBaseURL(ctx, modelID, &cfg); err != nil {
		return nil, err
	}

	client, err := openaibedrock.NewClient(ctx, cfg, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("bedrock mantle: create client: %w", err)
	}
	return openaiprovider.NewResponsesWithClient(
		client,
		modelID,
		openaiprovider.WithProviderName(responsesProviderName),
	), nil
}

func applyDefaultBaseURL(ctx context.Context, modelID string, cfg *Config) error {
	if cfg.BaseURL != "" || strings.TrimSpace(os.Getenv("AWS_BEDROCK_BASE_URL")) != "" {
		return nil
	}

	region := cfg.AWSRegion
	if region == "" {
		loadOptions := make([]func(*awsconfig.LoadOptions) error, 0, 1)
		if cfg.AWSProfile != "" {
			loadOptions = append(loadOptions, awsconfig.WithSharedConfigProfile(cfg.AWSProfile))
		}
		awsConfig, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
		if err != nil {
			return fmt.Errorf("bedrock mantle: resolve AWS region: %w", err)
		}
		region = awsConfig.Region
	}
	if strings.TrimSpace(region) == "" {
		return fmt.Errorf("bedrock mantle: AWS region is required for endpoint resolution")
	}

	cfg.AWSRegion = region
	cfg.BaseURL = defaultBaseURL(region, modelID)
	return nil
}

func defaultBaseURL(region, modelID string) string {
	path := "/v1"
	if modelID == gpt56LunaModelID {
		path = "/openai/v1"
	}
	return fmt.Sprintf("https://bedrock-mantle.%s.api.aws%s", region, path)
}
