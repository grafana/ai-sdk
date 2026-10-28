package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

const exampleRoutePrefix = "https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123"

// An internal service exchanges its access policy token for one addressed to
// the Gateway plugin and uses the configured models.
func ExampleNewWithTokenExchange() {
	client, err := NewWithTokenExchange(TokenExchangeConfig{
		CAPToken:         os.Getenv("GRAFANA_CAP_TOKEN"),
		TokenExchangeURL: os.Getenv("GRAFANA_TOKEN_EXCHANGE_URL"),
		Namespace:        "stacks-123",
		Audience:         "grafana-aigateway-app",
		BaseURL:          exampleRoutePrefix,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	models, err := client.ListModels(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, model := range models {
		fmt.Println(model.ID)
	}
}

// An internal service calls on behalf of a user. The subject is the user's
// Grafana ID token, or the delegated access token a plugin received for them.
func ExampleWithOnBehalfOf() {
	client, err := NewWithTokenExchange(TokenExchangeConfig{
		CAPToken:         os.Getenv("GRAFANA_CAP_TOKEN"),
		TokenExchangeURL: os.Getenv("GRAFANA_TOKEN_EXCHANGE_URL"),
		Namespace:        "stacks-123",
		Audience:         "grafana-aigateway-app",
		BaseURL:          exampleRoutePrefix,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	model, err := client.LanguageModel("grafana/assistant")
	if err != nil {
		fmt.Println(err)
		return
	}
	ctx := WithOnBehalfOf(context.Background(), os.Getenv("USER_ID_TOKEN"))
	result, err := aisdk.GenerateText(ctx, model, aisdk.WithModelMessages(provider.UserText("Summarize this incident.")))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(result.Text)
}

// A service account token calls with request-supplied provider keys in
// Grafana Cloud. In self-hosted Grafana, the same client uses the configured
// models instead and sends no provider keys.
func ExampleNewWithServiceAccountToken() {
	client, err := NewWithServiceAccountToken(ServiceAccountConfig{
		Token:   os.Getenv("GRAFANA_SERVICE_ACCOUNT_TOKEN"),
		BaseURL: exampleRoutePrefix,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	model, err := client.LanguageModel("openai/gpt-5")
	if err != nil {
		fmt.Println(err)
		return
	}
	result, err := aisdk.GenerateText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("Summarize this incident.")),
		aisdk.WithProviderOptions(GatewayOptions{
			BYOK: map[string][]BYOKAccount{"openai": {{APIKey: os.Getenv("OPENAI_API_KEY")}}},
		}),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(result.Text)
}

// Routing controls try fallback models and prefer or restrict providers for
// one call. The routing metadata reports which models and providers were tried.
func ExampleGatewayOptions() {
	client, err := NewWithServiceAccountToken(ServiceAccountConfig{
		Token:   os.Getenv("GRAFANA_SERVICE_ACCOUNT_TOKEN"),
		BaseURL: exampleRoutePrefix,
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	model, err := client.LanguageModel("grafana/assistant")
	if err != nil {
		fmt.Println(err)
		return
	}
	result, err := aisdk.GenerateText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("Summarize this incident.")),
		aisdk.WithProviderOptions(GatewayOptions{
			Models: []string{"grafana/backup"},
			Order:  []string{"anthropic"},
		}),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	gateway, err := GatewayMetadataFrom(result.ProviderMetadata)
	if err != nil || gateway == nil || gateway.Routing == nil {
		fmt.Println("no routing metadata")
		return
	}
	for _, model := range gateway.Routing.ModelAttempts {
		fmt.Println(model.ModelID, model.Success)
	}
}

// GatewayMetadataFromError reads the attempts of a failed call.
func ExampleGatewayMetadataFromError() {
	failed := provider.NewAPICallError(provider.APICallErrorOptions{
		Message: "service overloaded",
		Data:    json.RawMessage(`{"providerMetadata":{"gateway":{"execution":{"requestedModelId":"assistant","attempts":[{"provider":"anthropic","modelId":"claude","outcome":"failed","error":{"message":"overloaded","statusCode":529}}]}}}}`),
	})
	gateway, err := GatewayMetadataFromError(failed)
	if err != nil || gateway == nil {
		fmt.Println("no gateway metadata")
		return
	}
	for _, attempt := range gateway.Execution.Attempts {
		fmt.Println(attempt.Provider, attempt.Outcome, attempt.Error.StatusCode)
	}
	// Output: anthropic failed 529
}
