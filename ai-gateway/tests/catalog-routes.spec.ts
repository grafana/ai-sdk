import { createHash } from 'node:crypto';
import type { APIRequestContext, APIResponse } from '@playwright/test';
import { test, expect, ROLES } from './fixtures';
import { API_GROUP, API_VERSION } from '../src/constants';

const PREFIX = `/apis/${API_GROUP}/${API_VERSION}/namespaces/default`;
const CATALOGS = `${PREFIX}/modelcatalogs`;
const SENTINEL = 'e2e-catalog-sentinel-key';
const KEY_DIGEST_LENGTH = 12;
const PROMPT = { prompt: [{ role: 'user', content: [{ type: 'text', text: 'hi' }] }], maxOutputTokens: 16 };

let sequence = 0;

function uniqueName(): string {
  sequence += 1;
  return `e2e-routes-${process.pid}-${Date.now()}-${sequence}`;
}

function catalog(name: string, primary: string, key: string) {
  return {
    apiVersion: `${API_GROUP}/${API_VERSION}`,
    kind: 'ModelCatalog',
    metadata: { name },
    spec: {
      providers: { claude: { type: 'anthropic' } },
      models: {
        'team-model': { name: 'Team model', aliases: ['team'], primary: { provider: 'claude', model: primary } },
      },
    },
    secure: { claude: { create: key } },
  };
}

// nativeReply is the text the fake provider answers a native call with: the
// start of the SHA-256 of the key it received, and the backend model.
function nativeReply(key: string, model: string): string {
  return `native ${createHash('sha256').update(key).digest('hex').slice(0, KEY_DIGEST_LENGTH)} ${model}`;
}

function callCatalog(request: APIRequestContext, name: string, modelID: string, streaming: boolean) {
  return request.post(`${CATALOGS}/${name}/language-model`, {
    headers: {
      'ai-language-model-specification-version': '4',
      'ai-language-model-id': modelID,
      'ai-language-model-streaming': String(streaming),
    },
    data: PROMPT,
  });
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

// expectStatus checks Grafana's own refusal: a Kubernetes Status, which
// Grafana writes without its type metadata, never a Gateway error document.
async function expectStatus(response: APIResponse, code: number, reason: string) {
  expect(response.status()).toBe(code);
  const status = await response.json();
  expect(status).toEqual(expect.objectContaining({ status: 'Failure', reason, code }));
  expect(status.error).toBeUndefined();
}

test.describe('ModelCatalog routes', () => {
  test.afterEach(async ({ accounts }) => {
    const list = await accounts.Admin.get(CATALOGS);
    for (const item of (await list.json()).items ?? []) {
      if (item.metadata.name.startsWith(`e2e-routes-${process.pid}-`)) {
        await accounts.Admin.delete(`${CATALOGS}/${item.metadata.name}`);
      }
    }
  });

  test('the invoker role is registered and bound to Editor', async ({ accounts }) => {
    const routeActions = ['language-model', 'config'].map((route) => `${API_GROUP}/modelcatalogs/${route}:get`);
    for (const role of ROLES) {
      const held = Object.keys(
        await (await accounts[role].get('/api/access-control/user/permissions?reloadcache=true')).json()
      );
      if (role === 'Viewer') {
        for (const action of routeActions) {
          expect(held).not.toContain(action);
        }
      } else {
        expect(held).toEqual(expect.arrayContaining(routeActions));
      }
    }
  });

  test('Viewers are refused and Editors and Admins discover and invoke a catalog with its key', async ({
    accounts,
  }) => {
    const name = uniqueName();
    const key = `${SENTINEL}-route-first`;
    expect((await accounts.Admin.post(CATALOGS, { data: catalog(name, 'claude-native', key) })).status()).toBe(201);

    await expectStatus(await accounts.Viewer.get(`${CATALOGS}/${name}/config`), 403, 'Forbidden');
    await expectStatus(await callCatalog(accounts.Viewer, name, 'team', false), 403, 'Forbidden');

    for (const role of ['Editor', 'Admin'] as const) {
      const config = await accounts[role].get(`${CATALOGS}/${name}/config`);
      expect(config.status()).toBe(200);
      const document = await config.text();
      expect(document).not.toContain(SENTINEL);
      expect(JSON.parse(document).models).toEqual([
        {
          id: 'team-model',
          name: 'Team model',
          specification: { specificationVersion: 'v4', provider: 'grafana', modelId: 'team-model' },
          gateway: {
            aliases: ['team'],
            primary: { providerInstance: 'claude', provider: 'anthropic', providerModelId: 'claude-native' },
            fallbacks: [],
          },
        },
      ]);
      for (const streaming of [false, true]) {
        const response = await callCatalog(accounts[role], name, 'team', streaming);
        expect(response.status()).toBe(200);
        expect(await replyText(response, streaming)).toBe(nativeReply(key, 'claude-native'));
      }
    }
  });

  test('a catalog edit applies on the next call', async ({ accounts }) => {
    const name = uniqueName();
    const first = `${SENTINEL}-route-before-edit`;
    const second = `${SENTINEL}-route-after-edit`;
    expect((await accounts.Admin.post(CATALOGS, { data: catalog(name, 'claude-native', first) })).status()).toBe(201);
    expect(await replyText(await callCatalog(accounts.Editor, name, 'team', false), false)).toBe(
      nativeReply(first, 'claude-native')
    );

    const stored = await (await accounts.Admin.get(`${CATALOGS}/${name}`)).json();
    const edited = { ...catalog(name, 'claude-edited', second), metadata: stored.metadata };
    expect((await accounts.Admin.put(`${CATALOGS}/${name}`, { data: edited })).status()).toBe(200);
    expect(await replyText(await callCatalog(accounts.Editor, name, 'team', false), false)).toBe(
      nativeReply(second, 'claude-edited')
    );
  });

  test('an unknown catalog is answered by Grafana with a 404 Status', async ({ accounts }) => {
    await expectStatus(await accounts.Editor.get(`${CATALOGS}/e2e-routes-missing/config`), 404, 'NotFound');
    await expectStatus(await callCatalog(accounts.Editor, 'e2e-routes-missing', 'team', false), 404, 'NotFound');
  });

  test('a stored catalog leaves the namespace routes unchanged', async ({ accounts, page }) => {
    const name = uniqueName();
    expect(
      (
        await accounts.Admin.post(CATALOGS, { data: catalog(name, 'claude-native', `${SENTINEL}-route-namespace`) })
      ).status()
    ).toBe(201);
    const config = await page.request.get(`${PREFIX}/config`);
    expect(config.status()).toBe(200);
    const document = await config.text();
    expect(document).toContain('e2e/fake');
    expect(document).not.toContain('team-model');
  });
});
