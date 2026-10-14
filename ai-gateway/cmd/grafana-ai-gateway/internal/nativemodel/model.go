package nativemodel

import (
	"net/http"

	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

const (
	AnthropicBaseURL = "https://api.anthropic.com"
	OpenAIBaseURL    = "https://api.openai.com/v1"
)

type Config struct {
	APIKey       string `json:"apiKey"`
	BaseURL      string `json:"baseURL,omitempty"`
	Organization string `json:"organization,omitempty"`
	Project      string `json:"project,omitempty"`
}

func NewAnthropic(config Config, modelID string, client *http.Client) provider.LanguageModel {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = AnthropicBaseURL
	}
	return anthropicprovider.New(config.APIKey, modelID, anthropicprovider.WithRequestOptions(
		anthropicoption.WithBaseURL(baseURL),
		anthropicoption.WithHTTPClient(client),
		anthropicoption.WithMaxRetries(0),
	))
}

func NewOpenAI(config Config, modelID string, client *http.Client) provider.LanguageModel {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = OpenAIBaseURL
	}
	options := []openaioption.RequestOption{
		openaioption.WithAPIKey(config.APIKey),
		openaioption.WithBaseURL(baseURL),
		openaioption.WithHTTPClient(client),
		openaioption.WithMaxRetries(0),
	}
	if config.Organization != "" {
		options = append(options, openaioption.WithOrganization(config.Organization))
	}
	if config.Project != "" {
		options = append(options, openaioption.WithProject(config.Project))
	}
	service := responses.NewResponseService(options...)
	return openaiprovider.NewResponsesWithClient(openaisdk.Client{Responses: service}, modelID)
}
