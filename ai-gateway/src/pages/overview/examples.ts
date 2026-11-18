export const EXAMPLE_MODEL = 'anthropic/claude-sonnet-5';

export enum Language {
  TypeScript = 'typescript',
  Go = 'go',
}

export const LANGUAGES = [
  { value: Language.TypeScript, label: 'TypeScript' },
  { value: Language.Go, label: 'Go' },
];

export const INSTALL: Record<Language, string> = {
  [Language.TypeScript]: 'npm install ai @ai-sdk/gateway',
  [Language.Go]: 'go get github.com/grafana/ai-sdk github.com/grafana/ai-sdk/providers/grafana',
};

export const ENVIRONMENT = `export GRAFANA_SERVICE_ACCOUNT_TOKEN=<service account token>
export ANTHROPIC_API_KEY=<your Anthropic key>`;

function typeScriptExample(endpoint: string): string {
  return `import { createGateway } from '@ai-sdk/gateway';
import { generateText } from 'ai';

const gateway = createGateway({
  baseURL: '${endpoint}',
  apiKey: process.env.GRAFANA_SERVICE_ACCOUNT_TOKEN,
});

const { text } = await generateText({
  model: gateway('${EXAMPLE_MODEL}'),
  prompt: 'Explain goroutines in one sentence.',
  providerOptions: {
    gateway: { byok: { anthropic: [{ apiKey: process.env.ANTHROPIC_API_KEY }] } },
  },
});

console.log(text);`;
}

function goExample(endpoint: string): string {
  return `package main

import (
	"context"
	"fmt"
	"log"
	"os"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
)

func main() {
	client, err := grafana.NewWithServiceAccountToken(grafana.ServiceAccountConfig{
		Token:   os.Getenv("GRAFANA_SERVICE_ACCOUNT_TOKEN"),
		BaseURL: "${endpoint}",
	})
	if err != nil {
		log.Fatal(err)
	}

	model, err := client.LanguageModel("${EXAMPLE_MODEL}")
	if err != nil {
		log.Fatal(err)
	}

	result, err := aisdk.GenerateText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("Explain goroutines in one sentence.")),
		aisdk.WithProviderOptions(grafana.GatewayOptions{
			BYOK: map[string][]grafana.BYOKAccount{
				"anthropic": {{APIKey: os.Getenv("ANTHROPIC_API_KEY")}},
			},
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Text)
}`;
}

export function callExample(language: Language, endpoint: string): string {
  return language === Language.TypeScript ? typeScriptExample(endpoint) : goExample(endpoint);
}
