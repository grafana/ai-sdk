import type { APIRequestContext, APIResponse } from '@playwright/test';
import { expect, permissions, ROLES, type Role, test } from './accounts';
import { API_GROUP, API_VERSION } from '../src/constants';

const CATALOGS = `/apis/${API_GROUP}/${API_VERSION}/namespaces/default/modelcatalogs`;
const SENTINEL = 'e2e-catalog-sentinel-key';
const INLINE_SECRET_NAME = /^inline-[a-z0-9]+$/;

type Secure = Record<string, { create?: string; name?: string; remove?: boolean }>;
type Providers = Record<string, Record<string, unknown>>;

let sequence = 0;

function uniqueName(): string {
  sequence += 1;
  return `e2e-${process.pid}-${Date.now()}-${sequence}`;
}

const twoProviders: Providers = { anthropic: { type: 'anthropic' }, openai: { type: 'openai' } };

function catalog(name: string, providers: Providers, secure?: Secure, annotations?: Record<string, string>) {
  const models: Record<string, unknown> = {};
  for (const provider of Object.keys(providers)) {
    models[`model-${provider}`] = { name: 'Model', primary: { provider, model: 'native' } };
  }
  return {
    apiVersion: `${API_GROUP}/${API_VERSION}`,
    kind: 'ModelCatalog',
    metadata: { name, ...(annotations ? { annotations } : {}) },
    spec: { providers, models },
    ...(secure ? { secure } : {}),
  };
}

function createKeys(providers: Providers): Secure {
  return Object.fromEntries(
    Object.keys(providers).map((provider) => [provider, { create: `${SENTINEL}-${provider}` }])
  );
}

async function expectNoKey(response: APIResponse) {
  expect(await response.text()).not.toContain(SENTINEL);
}

async function create(request: APIRequestContext, body: object): Promise<APIResponse> {
  const response = await request.post(CATALOGS, { data: body });
  await expectNoKey(response);
  return response;
}

async function update(request: APIRequestContext, name: string, body: object): Promise<APIResponse> {
  const response = await request.put(`${CATALOGS}/${name}`, { data: body });
  await expectNoKey(response);
  return response;
}

async function createStored(request: APIRequestContext) {
  const name = uniqueName();
  const response = await create(request, catalog(name, twoProviders, createKeys(twoProviders)));
  expect(response.status()).toBe(201);
  return { name, object: await response.json() };
}

async function expectRefused(response: APIResponse, field: string) {
  expect(response.status()).toBe(400);
  const status = await response.json();
  expect(status.message).toContain('admission hook for ModelCatalog denied the request');
  expect(status.message).toContain(field);
}

test.describe('ModelCatalog', () => {
  test.afterEach(async ({ accounts }) => {
    const list = await accounts.Admin.get(CATALOGS);
    for (const item of (await list.json()).items ?? []) {
      if (item.metadata.name.startsWith(`e2e-${process.pid}-`)) {
        await accounts.Admin.delete(`${CATALOGS}/${item.metadata.name}`);
      }
    }
  });

  test('catalog roles are registered and bound to Viewer and Admin', async ({ accounts }) => {
    const actions = (role: Role) => permissions(accounts[role]);
    const read = `${API_GROUP}/modelcatalogs:get`;
    const writes = ['create', 'update', 'delete'].map((verb) => `${API_GROUP}/modelcatalogs:${verb}`);
    for (const role of ['Viewer', 'Editor'] as const) {
      const held = await actions(role);
      expect(held).toContain(read);
      for (const write of writes) {
        expect(held).not.toContain(write);
      }
    }
    const admin = await actions('Admin');
    expect(admin).toEqual(expect.arrayContaining([read, ...writes]));

    for (const role of ['Viewer', 'Editor'] as const) {
      expect((await accounts[role].get(CATALOGS)).status()).toBe(200);
      const refused = await create(accounts[role], catalog(uniqueName(), twoProviders, createKeys(twoProviders)));
      expect(refused.status()).toBe(403);
    }
    const { name } = await createStored(accounts.Admin);
    expect((await accounts.Viewer.get(`${CATALOGS}/${name}`)).status()).toBe(200);
  });

  test('a catalog saves without a folder and refuses one', async ({ accounts }) => {
    const { object } = await createStored(accounts.Admin);
    expect(object.metadata.annotations?.['grafana.app/folder']).toBeUndefined();
    const foldered = await create(
      accounts.Admin,
      catalog(uniqueName(), twoProviders, createKeys(twoProviders), { 'grafana.app/folder': 'e2e-folder' })
    );
    expect(foldered.status()).toBeGreaterThanOrEqual(400);
  });

  test('secure.create stores a secret and reads return names only', async ({ accounts }) => {
    const { name, object } = await createStored(accounts.Admin);
    for (const provider of Object.keys(twoProviders)) {
      expect(Object.keys(object.secure[provider])).toEqual(['name']);
      expect(object.secure[provider].name).toMatch(INLINE_SECRET_NAME);
    }
    expect(object.secure.anthropic.name).not.toBe(object.secure.openai.name);
    const read = await accounts.Viewer.get(`${CATALOGS}/${name}`);
    await expectNoKey(read);
    expect((await read.json()).secure).toEqual(object.secure);
    await expectNoKey(await accounts.Viewer.get(CATALOGS));
  });

  test('an update that omits secure keeps the stored keys', async ({ accounts }) => {
    const { name, object } = await createStored(accounts.Admin);
    const changed = catalog(name, twoProviders);
    changed.spec.models['model-anthropic'] = { name: 'Renamed', primary: { provider: 'anthropic', model: 'native' } };
    const updated = await update(accounts.Admin, name, { ...changed, metadata: object.metadata });
    expect(updated.status()).toBe(200);
    expect((await updated.json()).secure).toEqual(object.secure);

    const current = await (await accounts.Admin.get(`${CATALOGS}/${name}`)).json();
    const roundTrip = await update(accounts.Admin, name, current);
    expect(roundTrip.status()).toBe(200);
    expect((await roundTrip.json()).secure).toEqual(object.secure);
  });

  test('removing a key a provider still uses is refused', async ({ accounts }) => {
    const { name, object } = await createStored(accounts.Admin);
    const removed = await update(accounts.Admin, name, {
      ...catalog(name, twoProviders, { openai: { remove: true } }),
      metadata: object.metadata,
    });
    await expectRefused(removed, 'providers.openai requires the secure value secure.openai');

    const withoutOpenAI = { anthropic: twoProviders.anthropic };
    const dropped = await update(accounts.Admin, name, {
      ...catalog(name, withoutOpenAI, { openai: { remove: true } }),
      metadata: object.metadata,
    });
    expect(dropped.status()).toBe(200);
    expect(Object.keys((await dropped.json()).secure)).toEqual(['anthropic']);
  });

  test('baseURL and name references are refused', async ({ accounts }) => {
    const endpoint = {
      anthropic: { type: 'anthropic', baseURL: 'https://backend.invalid' },
      openai: { type: 'openai' },
    };
    await expectRefused(
      await create(accounts.Admin, catalog(uniqueName(), endpoint, createKeys(endpoint))),
      'providers.anthropic.baseURL'
    );
    await expectRefused(
      await create(
        accounts.Admin,
        catalog(uniqueName(), twoProviders, {
          anthropic: { create: `${SENTINEL}-anthropic` },
          openai: { name: 'shared-secret' },
        })
      ),
      'secure.openai must not reference a secret by name'
    );

    const { name, object } = await createStored(accounts.Admin);
    const swapped = { anthropic: { name: object.secure.openai.name }, openai: { name: object.secure.anthropic.name } };
    await expectRefused(
      await update(accounts.Admin, name, { ...catalog(name, twoProviders, swapped), metadata: object.metadata }),
      'must not reference a secret by name'
    );
  });

  test('a stored catalog changes nothing on the namespace routes', async ({ accounts }) => {
    await createStored(accounts.Admin);
    for (const role of ROLES) {
      const config = await accounts[role].get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/default/config`);
      expect(config.status()).toBe(200);
      const body = await config.text();
      expect(body).toContain('anthropic/claude-sonnet-5');
      expect(body).not.toContain('model-anthropic');
    }
  });

  test('validator errors never contain key values', async ({ accounts }) => {
    const invalid = [
      catalog(uniqueName(), { Anthropic: { type: 'anthropic' } }, { Anthropic: { create: SENTINEL } }),
      catalog(uniqueName(), { compatible: { type: 'openai-compatible' } }, { compatible: { create: SENTINEL } }),
      catalog(uniqueName(), twoProviders, { anthropic: { create: SENTINEL } }),
      catalog(uniqueName(), twoProviders, { ...createKeys(twoProviders), extra: { create: SENTINEL } }),
    ];
    for (const body of invalid) {
      const response = await create(accounts.Admin, body);
      expect(response.status()).toBeGreaterThanOrEqual(400);
    }
  });
});
