import React, { useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { config, PluginPage } from '@grafana/runtime';
import { Stack, Tab, TabContent, TabsBar, Text, TextLink, useStyles2 } from '@grafana/ui';
import { CodeSnippet } from '../components/CodeSnippet/CodeSnippet';
import { testIds } from '../components/testIds';
import { API_GROUP, API_VERSION } from '../constants';

const REPOSITORY_URL = 'https://github.com/grafana/ai-sdk';
const DOCS_URL = `${REPOSITORY_URL}/blob/main`;
const EXAMPLE_MODEL = 'anthropic/claude-sonnet-5';

const OUTCOMES = [
  { title: 'Authenticate callers', href: `${DOCS_URL}/docs/guides/gateway-authentication.md` },
  { title: 'Call the Gateway from Go', href: `${DOCS_URL}/docs/providers/grafana-gateway.md` },
  {
    title: 'Use your own provider keys',
    href: `${DOCS_URL}/docs/providers/grafana-gateway.md#use-your-own-provider-credentials`,
  },
  { title: 'Configure the model catalog', href: `${DOCS_URL}/ai-gateway/docs/model-catalog.md` },
  { title: 'Observe Gateway traffic', href: `${DOCS_URL}/ai-gateway/docs/text-observability.md` },
];

const RESOURCES = [
  { title: 'Gateway documentation', href: `${REPOSITORY_URL}/tree/main/ai-gateway` },
  { title: 'Go API reference', href: 'https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana' },
  { title: 'AI SDK for Go', href: 'https://grafana.ai/technology/ai-sdk' },
  { title: 'Source and contributing', href: REPOSITORY_URL },
];

enum Language {
  TypeScript = 'typescript',
  Go = 'go',
}

export function gatewayEndpoint(appUrl: string, namespace: string): string {
  return `${appUrl.replace(/\/+$/, '')}/apis/${API_GROUP}/${API_VERSION}/namespaces/${encodeURIComponent(namespace)}`;
}

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
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Text)
}`;
}

const INSTALL = {
  [Language.TypeScript]: 'npm install ai @ai-sdk/gateway',
  [Language.Go]: 'go get github.com/grafana/ai-sdk github.com/grafana/ai-sdk/providers/grafana',
};

function LinkList({ links }: { links: Array<{ title: string; href: string }> }) {
  const s = useStyles2(getStyles);

  return (
    <ul className={s.list}>
      {links.map((link) => (
        <li key={link.href}>
          <TextLink href={link.href} external>
            {link.title}
          </TextLink>
        </li>
      ))}
    </ul>
  );
}

function Overview() {
  const s = useStyles2(getStyles);
  const [language, setLanguage] = useState(Language.TypeScript);
  const endpoint = gatewayEndpoint(config.appUrl, config.namespace);
  const example = language === Language.TypeScript ? typeScriptExample(endpoint) : goExample(endpoint);

  return (
    <PluginPage subTitle="Call language models from Go and TypeScript through Grafana.">
      <div className={s.page} data-testid={testIds.overview.container}>
        <Stack direction="column" gap={4}>
          <Stack direction="column" gap={1}>
            <Text element="p">
              The AI Gateway serves language models behind Grafana authentication. Depending on how it is deployed,
              callers use the models configured for this Grafana instance or bring their own provider keys with each
              request.
            </Text>
            <Text element="p">
              It speaks the AI SDK language-model protocol, so Go services use the Grafana provider and TypeScript
              services use the AI SDK gateway provider, with the same streaming, tool calling and structured output as a
              direct provider.
            </Text>
          </Stack>

          <Stack direction="column" gap={1}>
            <Text element="h2" variant="h3">
              Endpoint
            </Text>
            <Text element="p" color="secondary">
              Point a client at this Grafana stack&apos;s Gateway routes:
            </Text>
            <div data-testid={testIds.overview.endpoint}>
              <CodeSnippet code={endpoint} />
            </div>
          </Stack>

          <Stack direction="column" gap={1}>
            <Text element="h2" variant="h3">
              Quick start
            </Text>
            <Text element="p" color="secondary">
              Create a service account token and call the Gateway from your server. Keep tokens and provider keys out of
              browser code. In Grafana Cloud, service accounts bring their own provider keys with each request.
            </Text>
            <TabsBar>
              <Tab
                label="TypeScript"
                active={language === Language.TypeScript}
                onChangeTab={() => setLanguage(Language.TypeScript)}
              />
              <Tab label="Go" active={language === Language.Go} onChangeTab={() => setLanguage(Language.Go)} />
            </TabsBar>
            <TabContent>
              <Stack direction="column" gap={1}>
                <CodeSnippet code={INSTALL[language]} />
                <CodeSnippet code={example} />
              </Stack>
            </TabContent>
          </Stack>

          <Stack direction="column" gap={1}>
            <Text element="h2" variant="h3">
              Start with your outcome
            </Text>
            <LinkList links={OUTCOMES} />
          </Stack>

          <Stack direction="column" gap={1}>
            <Text element="h2" variant="h3">
              Project resources
            </Text>
            <LinkList links={RESOURCES} />
          </Stack>
        </Stack>
      </div>
    </PluginPage>
  );
}

export default Overview;

const getStyles = (theme: GrafanaTheme2) => ({
  page: css({
    maxWidth: theme.breakpoints.values.lg,
  }),
  list: css({
    margin: 0,
    paddingLeft: theme.spacing(3),
    lineHeight: theme.typography.body.lineHeight * 1.4,
  }),
});
