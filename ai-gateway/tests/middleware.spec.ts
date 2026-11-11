import type { APIRequestContext, APIResponse } from '@playwright/test';
import { expect, permissions, test } from './accounts';
import { API_GROUP, API_VERSION } from '../src/constants';

const MIDDLEWARES = `/apis/${API_GROUP}/${API_VERSION}/namespaces/default/middlewares`;
const SENTINEL = 'e2e-middleware-sentinel-value';
const TYPE = 'e2e-unregistered';

const MEMBER = 'e2eUnregistered';
const LANGUAGE_MODEL = `/apis/${API_GROUP}/${API_VERSION}/namespaces/default/language-model`;
const FAKE_MODEL = 'e2e/fake';
const ECHO_MARKER = 'e2e-echo-request';
const INSTRUCTIONS = 'default-instructions';
const SETTINGS = 'default-settings';
const INSTRUCTIONS_MEMBER = { messages: [{ content: 'Be brief.' }, { content: 'Cite sources.' }] };
const SEED = '9007199254740993';
const SETTINGS_MEMBER_JSON = `{"temperature":0.25,"maxOutputTokens":48,"stopSequences":["END"],"seed":${SEED}}`;
const PROPAGATION = { timeout: 70_000, intervals: [2_000] };
const RUNTIME_TEST_TIMEOUT = 240_000;

function middleware(name: string, type: string, members: Record<string, unknown> = {}) {
  return {
    apiVersion: `${API_GROUP}/${API_VERSION}`,
    kind: 'Middleware',
    metadata: { name },
    spec: { type, enabled: true, ...members },
  };
}

async function expectNoSentinel(response: APIResponse) {
  expect(await response.text()).not.toContain(SENTINEL);
}

async function create(request: APIRequestContext, body: object): Promise<APIResponse> {
  const response = await request.post(MIDDLEWARES, { data: body });
  await expectNoSentinel(response);
  return response;
}

async function expectRefusedByGrafana(response: APIResponse) {
  expect([400, 422]).toContain(response.status());
  expect((await response.json()).message).not.toContain('admission hook for Middleware');
}

async function stored(request: APIRequestContext, name: string) {
  const response = await request.get(`${MIDDLEWARES}/${name}`);
  expect(response.status(), await response.text()).toBe(200);
  return { text: await response.text(), object: await response.json() };
}

async function update(request: APIRequestContext, name: string, spec: object): Promise<APIResponse> {
  const { object } = await stored(request, name);
  const response = await request.put(`${MIDDLEWARES}/${name}`, { data: { ...object, spec } });
  await expectNoSentinel(response);
  return response;
}

async function remove(request: APIRequestContext, name: string) {
  expect([200, 404]).toContain((await request.delete(`${MIDDLEWARES}/${name}`)).status());
}

type Echo = { system: string[]; temperature: number | null; maxTokens: number | null; stopSequences: string[] | null };

async function echo(request: APIRequestContext): Promise<Echo> {
  const response = await request.post(LANGUAGE_MODEL, {
    headers: {
      'ai-language-model-specification-version': '4',
      'ai-language-model-id': FAKE_MODEL,
      'ai-language-model-streaming': 'false',
    },
    data: { prompt: [{ role: 'user', content: [{ type: 'text', text: ECHO_MARKER }] }] },
  });
  expect(response.status(), await response.text()).toBe(200);
  const received = JSON.parse((await response.json()).content[0].text);
  return {
    system: (received.system ?? []).map((block: { text: string }) => block.text),
    temperature: received.temperature,
    maxTokens: received.max_tokens,
    stopSequences: received.stop_sequences,
  };
}

async function expectRefused(response: APIResponse, reason: string) {
  expect(response.status()).toBe(400);
  const status = await response.json();
  expect(status.message).toContain('admission hook for Middleware denied the request');
  expect(status.message).toContain(reason);
}

test.describe('Middleware', () => {
  test('middleware roles are registered and bound to Viewer and Admin', async ({ accounts }) => {
    const read = `${API_GROUP}/middlewares:get`;
    const writes = ['create', 'update', 'delete'].map((verb) => `${API_GROUP}/middlewares:${verb}`);
    for (const role of ['Viewer', 'Editor'] as const) {
      const held = await permissions(accounts[role]);
      expect(held).toContain(read);
      for (const write of writes) {
        expect(held).not.toContain(write);
      }
    }
    expect(await permissions(accounts.Admin)).toEqual(expect.arrayContaining([read, ...writes]));
  });

  test('Viewers and Editors read, and their creates are refused', async ({ accounts }) => {
    for (const role of ['Viewer', 'Editor'] as const) {
      expect((await accounts[role].get(MIDDLEWARES)).status()).toBe(200);
      const created = await create(accounts[role], middleware(TYPE, TYPE));
      expect([400, 403]).toContain(created.status());
      expect((await accounts[role].get(`${MIDDLEWARES}/${TYPE}`)).status()).toBe(404);
    }
  });

  test('Admin writes reach admission, which refuses unregistered types and mismatched names', async ({ accounts }) => {
    await expectRefused(
      await create(accounts.Admin, middleware(TYPE, TYPE)),
      `spec.type "${TYPE}" is not a tenant extension of this Gateway`
    );
    await expectRefused(
      await create(accounts.Admin, middleware(`${TYPE}-other`, TYPE)),
      'metadata.name must equal spec.type'
    );
    expect((await accounts.Admin.get(`${MIDDLEWARES}/${TYPE}`)).status()).toBe(404);
    expect((await accounts.Admin.delete(`${MIDDLEWARES}/${TYPE}`)).status()).toBe(404);
  });

  test('Grafana refuses members the manifest does not declare without echoing their values', async ({ accounts }) => {
    const refused = await create(
      accounts.Admin,
      middleware(TYPE, TYPE, { [MEMBER]: { instructions: SENTINEL, nested: { value: SENTINEL } } })
    );
    expect([400, 422]).toContain(refused.status());
    expect((await refused.json()).message).not.toContain('admission hook for Middleware');
    expect((await accounts.Admin.get(`${MIDDLEWARES}/${TYPE}`)).status()).toBe(404);
  });
});

test.describe('Middleware with the shipped types', () => {
  test.describe.configure({ mode: 'serial' });

  test.beforeAll(async ({ accounts }) => {
    await remove(accounts.Admin, INSTRUCTIONS);
    await remove(accounts.Admin, SETTINGS);
  });

  test.afterAll(async ({ accounts }) => {
    await remove(accounts.Admin, INSTRUCTIONS);
    await remove(accounts.Admin, SETTINGS);
  });

  test('Grafana refuses invalid members without echoing their values', async ({ accounts }) => {
    for (const [name, member] of [
      [SETTINGS, { defaultSettings: { temperature: SENTINEL } }],
      [SETTINGS, { defaultSettings: { headers: { 'x-tenant': SENTINEL } } }],
      [INSTRUCTIONS, { defaultInstructions: { messages: [{ content: SENTINEL }, {}] } }],
      [INSTRUCTIONS, { defaultInstructions: { messages: [{ content: 'Be brief.', note: SENTINEL }] } }],
    ] as const) {
      await expectRefusedByGrafana(await create(accounts.Admin, middleware(name, name, member)));
      expect((await accounts.Admin.get(`${MIDDLEWARES}/${name}`)).status()).toBe(404);
    }
  });

  test('admission refuses values outside the schema, naming the location', async ({ accounts }) => {
    await expectRefused(
      await create(
        accounts.Admin,
        middleware(INSTRUCTIONS, INSTRUCTIONS, {
          defaultInstructions: { messages: [{ content: SENTINEL }, { content: '' }] },
        })
      ),
      "spec.defaultInstructions does not match the extension's schema at /messages/1/content"
    );
    expect((await accounts.Admin.get(`${MIDDLEWARES}/${INSTRUCTIONS}`)).status()).toBe(404);
  });

  test('a folder annotation is refused, and a resource without one is stored', async ({ accounts }) => {
    const inFolder = middleware(SETTINGS, SETTINGS, { defaultSettings: { temperature: 0.25 } });
    const refused = await create(accounts.Admin, {
      ...inFolder,
      metadata: { ...inFolder.metadata, annotations: { 'grafana.app/folder': 'e2e-folder' } },
    });
    expect(refused.status(), await refused.text()).toBeGreaterThanOrEqual(400);
    expect((await accounts.Admin.get(`${MIDDLEWARES}/${SETTINGS}`)).status()).toBe(404);
    expect((await create(accounts.Admin, inFolder)).status()).toBe(201);
    await remove(accounts.Admin, SETTINGS);
  });

  test('stored resources apply to calls until disabled or deleted, and only Admins change them', async ({
    accounts,
    page,
  }) => {
    test.setTimeout(RUNTIME_TEST_TIMEOUT);
    const instructions = await create(
      accounts.Admin,
      middleware(INSTRUCTIONS, INSTRUCTIONS, { defaultInstructions: INSTRUCTIONS_MEMBER })
    );
    expect(instructions.status(), await instructions.text()).toBe(201);
    const settings = await accounts.Admin.post(MIDDLEWARES, {
      headers: { 'Content-Type': 'application/json' },
      data: `{"apiVersion":"${API_GROUP}/${API_VERSION}","kind":"Middleware","metadata":{"name":"${SETTINGS}"},"spec":{"type":"${SETTINGS}","enabled":true,"defaultSettings":${SETTINGS_MEMBER_JSON}}}`,
    });
    expect(settings.status(), await settings.text()).toBe(201);

    expect((await stored(accounts.Admin, INSTRUCTIONS)).object.spec.defaultInstructions).toEqual(INSTRUCTIONS_MEMBER);
    expect((await stored(accounts.Admin, SETTINGS)).text).toContain(`"seed":${SEED}`);

    for (const role of ['Viewer', 'Editor'] as const) {
      const { object } = await stored(accounts[role], INSTRUCTIONS);
      const changed = await accounts[role].put(`${MIDDLEWARES}/${INSTRUCTIONS}`, {
        data: { ...object, spec: { ...object.spec, enabled: false } },
      });
      expect(changed.status(), role).toBe(403);
      expect((await accounts[role].delete(`${MIDDLEWARES}/${INSTRUCTIONS}`)).status(), role).toBe(403);
    }
    expect((await stored(accounts.Admin, INSTRUCTIONS)).object.spec.enabled).toBe(true);

    await expect
      .poll(() => echo(page.request), PROPAGATION)
      .toEqual({ system: ['Be brief.', 'Cite sources.'], temperature: 0.25, maxTokens: 48, stopSequences: ['END'] });

    const updated = await update(accounts.Admin, SETTINGS, {
      type: SETTINGS,
      enabled: true,
      defaultSettings: { temperature: 0.5, maxOutputTokens: 48 },
    });
    expect(updated.status(), await updated.text()).toBe(200);
    await expectRefused(
      await update(accounts.Admin, SETTINGS, {
        type: SETTINGS,
        enabled: true,
        defaultSettings: { maxOutputTokens: 0, stopSequences: [SENTINEL] },
      }),
      "spec.defaultSettings does not match the extension's schema at /maxOutputTokens"
    );

    const disabled = await update(accounts.Admin, INSTRUCTIONS, {
      type: INSTRUCTIONS,
      enabled: false,
      defaultInstructions: INSTRUCTIONS_MEMBER,
    });
    expect(disabled.status(), await disabled.text()).toBe(200);
    expect((await accounts.Admin.delete(`${MIDDLEWARES}/${SETTINGS}`)).status()).toBe(200);
    expect((await accounts.Admin.get(`${MIDDLEWARES}/${SETTINGS}`)).status()).toBe(404);

    await expect
      .poll(async () => {
        const { system, temperature, stopSequences } = await echo(page.request);
        return { system, temperature, stopSequences };
      }, PROPAGATION)
      .toEqual({ system: [], temperature: null, stopSequences: null });
    expect((await echo(page.request)).maxTokens).not.toBe(48);
  });
});
