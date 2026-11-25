import { createHash } from 'node:crypto';
import type { APIRequestContext, APIResponse } from '@playwright/test';
import { test, expect } from './fixtures';
import { API_GROUP, API_VERSION } from '../src/constants';

// The e2e environment lists one custom endpoint, the fake provider's TLS
// listener under /team, as an operator would list a provider endpoint it
// trusts.
const CATALOGS = `/apis/${API_GROUP}/${API_VERSION}/namespaces/default/modelcatalogs`;
const SENTINEL = 'e2e-catalog-sentinel-key';
const KEY_DIGEST_LENGTH = 12;
const LISTED = 'https://fake-anthropic:8443/team';
const PROMPT = { prompt: [{ role: 'user', content: [{ type: 'text', text: 'hi' }] }], maxOutputTokens: 16 };

let sequence = 0;

function uniqueName(): string {
  sequence += 1;
  return `e2e-endpoints-${process.pid}-${Date.now()}-${sequence}`;
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

async function create(request: APIRequestContext, provider: Record<string, unknown>, key = `${SENTINEL}-endpoint`) {
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
// start of the SHA-256 of the key it received and the backend model.
function customReply(key: string): string {
  return `custom ${createHash('sha256').update(key).digest('hex').slice(0, KEY_DIGEST_LENGTH)} claude-custom`;
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

test.describe('ModelCatalog custom endpoints', () => {
  test.afterEach(async ({ accounts }) => {
    const list = await accounts.Admin.get(CATALOGS);
    for (const item of (await list.json()).items ?? []) {
      if (item.metadata.name.startsWith(`e2e-endpoints-${process.pid}-`)) {
        await accounts.Admin.delete(`${CATALOGS}/${item.metadata.name}`);
      }
    }
  });

  test('a listed endpoint serves unary and streamed calls with the catalog key', async ({ accounts }) => {
    const key = `${SENTINEL}-listed`;
    const { name, response } = await create(accounts.Admin, { type: 'anthropic', baseURL: LISTED }, key);
    expect(response.status()).toBe(201);
    for (const streaming of [false, true]) {
      const call = await callCatalog(accounts.Editor, name, streaming);
      expect(call.status()).toBe(200);
      expect(await replyText(call, streaming)).toBe(customReply(key));
    }
  });

  test('admission refuses endpoints outside the list', async ({ accounts }) => {
    for (const baseURL of [
      'https://unlisted.example',
      'https://fake-anthropic:8444/team',
      'http://fake-anthropic:8443/team',
      'https://fake-anthropic:8443/teams',
      'https://fake-anthropic:8443',
      'https://fake-anthropic:8443/team/../admin',
    ]) {
      const { response } = await create(accounts.Admin, { type: 'anthropic', baseURL });
      expect(response.status(), baseURL).toBe(400);
      const body = await response.text();
      expect(body).toContain('modelcatalog: providers.custom.baseURL');
      expect(body).not.toContain('fake-anthropic');
      expect(body).not.toContain('unlisted.example');
    }
  });

  test('an openai-compatible provider at a listed endpoint is accepted and discovered by its provider name', async ({
    accounts,
  }) => {
    const { name, response } = await create(accounts.Admin, {
      type: 'openai-compatible',
      baseURL: `${LISTED}/v1`,
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

  test('an explicit native endpoint needs no entry', async ({ accounts }) => {
    const { response } = await create(accounts.Admin, { type: 'anthropic', baseURL: 'https://API.anthropic.com/' });
    expect(response.status()).toBe(201);
  });
});
