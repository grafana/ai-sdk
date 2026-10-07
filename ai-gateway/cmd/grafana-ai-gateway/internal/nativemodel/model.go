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
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	defaultOpenAIBaseURL    = "https://api.openai.com/v1"
)

func NewAnthropic(apiKey, modelID, baseURL string, client *http.Client) provider.LanguageModel {
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	return anthropicprovider.New(apiKey, modelID, anthropicprovider.WithRequestOptions(
		anthropicoption.WithBaseURL(baseURL),
		anthropicoption.WithHTTPClient(client),
		anthropicoption.WithMaxRetries(0),
	))
}

func NewOpenAI(apiKey, modelID, baseURL string, client *http.Client) provider.LanguageModel {
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	service := responses.NewResponseService(
		openaioption.WithAPIKey(apiKey),
		openaioption.WithBaseURL(baseURL),
		openaioption.WithHTTPClient(client),
		openaioption.WithMaxRetries(0),
	)
	return openaiprovider.NewResponsesWithClient(openaisdk.Client{Responses: service}, modelID)
}
