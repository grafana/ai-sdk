const REPOSITORY_URL = 'https://github.com/grafana/ai-sdk';
const BLOB_URL = `${REPOSITORY_URL}/blob/main`;

export const LINKS = {
  repository: REPOSITORY_URL,
  gatewayGuide: `${BLOB_URL}/ai-gateway/README.md`,
  install: `${BLOB_URL}/ai-gateway/README.md#install`,
  configureModels: `${BLOB_URL}/ai-gateway/README.md#configure-models`,
  accessMode: `${BLOB_URL}/ai-gateway/README.md#choose-who-can-use-it`,
  storedCatalogs: `${BLOB_URL}/ai-gateway/README.md#store-model-catalogs-in-grafana`,
  limits: `${BLOB_URL}/ai-gateway/README.md#limits`,
  authentication: `${BLOB_URL}/docs/guides/gateway-authentication.md`,
  findGatewayURL: `${BLOB_URL}/docs/guides/gateway-authentication.md#find-your-gateway-url`,
  byok: `${BLOB_URL}/docs/guides/gateway-authentication.md#bring-your-own-provider-key-in-grafana-cloud`,
  grafanaService: `${BLOB_URL}/docs/guides/gateway-authentication.md#call-from-a-grafana-service-in-grafana-cloud`,
  appPlatform: `${BLOB_URL}/docs/guides/gateway-authentication.md#call-from-an-app-platform-app`,
  selfHostedCallers: `${BLOB_URL}/docs/guides/gateway-authentication.md#use-the-configured-models-in-self-hosted-grafana`,
  troubleshooting: `${BLOB_URL}/docs/guides/gateway-authentication.md#troubleshoot-authentication-errors`,
  goClient: `${BLOB_URL}/docs/providers/grafana-gateway.md`,
  providerCredentials: `${BLOB_URL}/docs/providers/grafana-gateway.md#use-your-own-provider-credentials`,
  monitoring: `${BLOB_URL}/ai-gateway/docs/text-observability.md`,
  architecture: `${BLOB_URL}/docs/concepts/gateway-architecture.md`,
  goReference: 'https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana',
  vercelGateway: 'https://ai-sdk.dev/providers/ai-sdk-providers/ai-gateway',
  serviceAccountDocs: 'https://grafana.com/docs/grafana/latest/administration/service-accounts/',
};

export const SERVICE_ACCOUNT_PATH = '/org/serviceaccounts/new';

export interface Caller {
  title: string;
  credential: string;
  models: string;
  href: string;
}

export const CALLERS: Caller[] = [
  {
    title: 'Your application',
    credential: 'A service account token from this stack.',
    models: 'Any model of your own Anthropic or OpenAI account, with your key on each request.',
    href: LINKS.byok,
  },
  {
    title: 'A Grafana service',
    credential: 'A Cloud access policy token, exchanged by the Go client.',
    models: 'The models Grafana configured for your service, once the Gateway owners allow it.',
    href: LINKS.grafanaService,
  },
  {
    title: 'An App Platform app',
    credential: 'The token exchange Grafana gives your plugin backend.',
    models: 'The configured models once your app is allowed; until then, your own key on behalf of a user.',
    href: LINKS.appPlatform,
  },
];

export interface Fact {
  term: string;
  description: string;
}

export const LIMITS: Fact[] = [
  { term: '60 seconds', description: 'per request, including a streamed response and every fallback attempt' },
  { term: '16 MiB', description: 'per request body' },
  { term: '8 attempts', description: 'per request, across fallback models or the provider keys you send' },
  { term: '30 seconds', description: 'without activity ends a stream' },
];

export const ERRORS: Fact[] = [
  { term: '401', description: 'The token is missing, expired or revoked.' },
  {
    term: '403 from Grafana',
    description: 'The token belongs to another stack, or the caller cannot use this app.',
  },
  {
    term: '403 Gateway permission error',
    description: 'A service calls as itself before the Gateway owners allow it.',
  },
  {
    term: '400 about gateway.byok',
    description: 'The request has no valid key for the selected provider. Send one with every call.',
  },
  {
    term: '400 about the selector',
    description: 'With your own key, select a model as provider/model, such as anthropic/claude-sonnet-5.',
  },
];

export interface Resource {
  title: string;
  description: string;
  href: string;
}

export const RESOURCES: Resource[] = [
  {
    title: 'Gateway guide',
    description: 'Install, configure models, choose who can use it, and limits.',
    href: LINKS.gatewayGuide,
  },
  {
    title: 'Authenticate to the Gateway',
    description: 'Credentials and models for each kind of caller.',
    href: LINKS.authentication,
  },
  {
    title: 'Call the Gateway from Go',
    description: 'Streaming, tools, fallback, routing and your own keys.',
    href: LINKS.goClient,
  },
  { title: 'Go API reference', description: 'The Grafana provider package on pkg.go.dev.', href: LINKS.goReference },
  {
    title: 'Vercel AI SDK Gateway provider',
    description: 'The TypeScript client the Gateway is compatible with.',
    href: LINKS.vercelGateway,
  },
  {
    title: 'Monitor the Gateway',
    description: 'Logs, metrics, traces and Agent Observability.',
    href: LINKS.monitoring,
  },
  {
    title: 'Architecture',
    description: 'The request path, component boundaries and glossary.',
    href: LINKS.architecture,
  },
  { title: 'Source and issues', description: 'Report a problem or contribute on GitHub.', href: LINKS.repository },
];
