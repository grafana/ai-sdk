import { createHash } from 'node:crypto';
import type { APIRequestContext, APIResponse } from '@playwright/test';
import { test, expect } from './fixtures';
import { API_GROUP, API_VERSION } from '../src/constants';

// The e2e environment allows the fake provider's hostname through the
// operator egress allow list, standing in for a public https endpoint. The
// same address is also reachable as egress-refused, which no entry allows,
// and as egress-denied, which the deny list refuses although it is allowed.
const CATALOGS = `/apis/${API_GROUP}/${API_VERSION}/namespaces/default/modelcatalogs`;
const SENTINEL = 'e2e-catalog-sentinel-key';
const KEY_DIGEST_LENGTH = 12;
const ALLOWED = 'https://fake-anthropic:8443';
const REFUSED = 'https://egress-refused:8443';
const DENIED = 'https://egress-denied:8443';
const PROMPT = { prompt: [{ role: 'user', content: [{ type: 'text', text: 'hi' }] }], maxOutputTokens: 16 };
const EGRESS_ERROR = {
  message: 'provider endpoint refused by the egress policy or not resolvable',
  type: 'failed_dependency',
  param: null,
  code: 'failed_dependency',
};
const ADDRESS = /\b\d{1,3}(\.\d{1,3}){3}\b|\[[0-9a-f:.]+\]/i;

let sequence = 0;

function uniqueName(): string {
  sequence += 1;
  return `e2e-egress-${process.pid}-${Date.now()}-${sequence}`;
}

function catalog(name: string, provider: Record<string, unknown>, key: string) {
  return {
    apiVersion: `${API_GROUP}/${API_VERSION}`,
    kind: 'ModelCatalog',
    metadata: { name },
    spec: {
      providers: { custom: provider },
      models: { 'custom-model': { name: 'Custom model', primary: { provider: 'custom', model: 'claude-custom' } } },
    },
    secure: { custom: { create: key } },
  };
}

async function create(request: APIRequestContext, provider: Record<string, unknown>, key = `${SENTINEL}-egress`) {
  const name = uniqueName();
  const response = await request.post(CATALOGS, { data: catalog(name, provider, key) });
  expect(await response.text()).not.toContain(SENTINEL);
  return { name, response };
}

function callCatalog(request: APIRequestContext, name: string, streaming: boolean) {
  return request.post(`${CATALOGS}/${name}/language-model`, {
    headers: {
      'ai-language-model-specification-version': '4',
      'ai-language-model-id': 'custom-model',
      'ai-language-model-streaming': String(streaming),
    },
    data: PROMPT,
  });
}

// customReply is the text the fake provider answers on its TLS listener: the
// start of the SHA-256 of the key it received, the backend model, and how
// many requests it received for the egress-refused name.
function customReply(key: string): string {
  return `custom ${createHash('sha256').update(key).digest('hex').slice(0, KEY_DIGEST_LENGTH)} claude-custom egress-refused=0`;
}

async function replyText(response: APIResponse, streaming: boolean): Promise<string> {
  const body = await response.text();
  expect(body).not.toContain(SENTINEL);
  if (!streaming) {
    return JSON.parse(body)
      .content.filter((part: { type: string }) => part.type === 'text')
      .map((part: { text: string }) => part.text)
      .join('');
  }
  const parts = body
    .split('\n')
    .filter((line) => line.startsWith('data: '))
    .map((line) => JSON.parse(line.slice('data: '.length)));
  expect(parts.at(-1)?.type).toBe('finish');
  return parts
    .filter((part) => part.type === 'text-delta')
    .map((part) => part.delta)
    .join('');
}

async function expectRefused(response: APIResponse, reason: string) {
  expect(response.status()).toBe(400);
  const body = await response.text();
  expect(body).toContain(`modelcatalog: providers.custom.baseURL ${reason}`);
  expect(body).not.toContain(SENTINEL);
}

test.describe('ModelCatalog custom endpoints', () => {
  test.afterEach(async ({ accounts }) => {
    const list = await accounts.Admin.get(CATALOGS);
    for (const item of (await list.json()).items ?? []) {
      if (item.metadata.name.startsWith(`e2e-egress-${process.pid}-`)) {
        await accounts.Admin.delete(`${CATALOGS}/${item.metadata.name}`);
      }
    }
  });

  test('an https endpoint the egress policy permits serves unary and streamed calls with the catalog key', async ({
    accounts,
  }) => {
    const key = `${SENTINEL}-allowed`;
    const { name, response } = await create(accounts.Admin, { type: 'anthropic', baseURL: ALLOWED }, key);
    expect(response.status()).toBe(201);
    for (const streaming of [false, true]) {
      const call = await callCatalog(accounts.Editor, name, streaming);
      expect(call.status()).toBe(200);
      expect(await replyText(call, streaming)).toBe(customReply(key));
    }
  });

  test('an endpoint resolving to a refused address fails without a request', async ({ accounts }) => {
    const { name: refused, response } = await create(accounts.Admin, { type: 'anthropic', baseURL: REFUSED });
    expect(response.status()).toBe(201);
    for (const streaming of [false, true]) {
      const call = await callCatalog(accounts.Editor, refused, streaming);
      expect(call.status()).toBe(424);
      const body = await call.text();
      expect(JSON.parse(body).error).toEqual(EGRESS_ERROR);
      expect(body).not.toContain('egress-refused');
      expect(body).not.toMatch(ADDRESS);
      expect(body).not.toContain(SENTINEL);
    }

    const key = `${SENTINEL}-after-refusal`;
    const { name: allowed } = await create(accounts.Admin, { type: 'anthropic', baseURL: ALLOWED }, key);
    expect(await replyText(await callCatalog(accounts.Editor, allowed, false), false)).toBe(customReply(key));
  });

  test('admission refuses plain HTTP, refused literals, numeric hosts and denied hosts', async ({ accounts }) => {
    for (const [baseURL, reason] of [
      ['http://fake-anthropic:8080', 'must use https'],
      ['https://127.0.0.1', 'is refused by the egress policy'],
      ['https://169.254.169.254', 'is refused by the egress policy'],
      ['https://[::ffff:10.0.0.1]', 'is refused by the egress policy'],
      ['https://2130706433', 'host must be a DNS name or an IP address'],
      [DENIED, 'is refused by the egress policy'],
    ]) {
      await expectRefused((await create(accounts.Admin, { type: 'anthropic', baseURL })).response, reason);
    }
  });

  test('an openai-compatible provider with its fields is accepted and discovered by its provider name', async ({
    accounts,
  }) => {
    const { name, response } = await create(accounts.Admin, {
      type: 'openai-compatible',
      baseURL: `${ALLOWED}/v1`,
      providerName: 'acme',
      supportsStructuredOutputs: true,
    });
    expect(response.status()).toBe(201);
    const config = await accounts.Editor.get(`${CATALOGS}/${name}/config`);
    expect(config.status()).toBe(200);
    const document = await config.text();
    expect(JSON.parse(document).models[0].gateway.primary).toEqual({
      providerInstance: 'custom',
      provider: 'acme',
      providerModelId: 'claude-custom',
    });
    expect(document).not.toContain('fake-anthropic');
  });
});
