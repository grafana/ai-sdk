import type { APIRequestContext, APIResponse } from '@playwright/test';
import { expect, permissions, test } from './accounts';
import { API_GROUP, API_VERSION } from '../src/constants';

const MIDDLEWARES = `/apis/${API_GROUP}/${API_VERSION}/namespaces/default/middlewares`;
const SENTINEL = 'e2e-middleware-sentinel-value';
const TYPE = 'e2e-unregistered';

function middleware(name: string, type: string, config: Record<string, unknown>) {
  return {
    apiVersion: `${API_GROUP}/${API_VERSION}`,
    kind: 'Middleware',
    metadata: { name },
    spec: { type, enabled: true, config },
  };
}

const sentinelConfig = { instructions: SENTINEL, [SENTINEL]: SENTINEL };

async function expectNoSentinel(response: APIResponse) {
  expect(await response.text()).not.toContain(SENTINEL);
}

async function create(request: APIRequestContext, body: object): Promise<APIResponse> {
  const response = await request.post(MIDDLEWARES, { data: body });
  await expectNoSentinel(response);
  return response;
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

  test('Viewers and Editors read but cannot write', async ({ accounts }) => {
    for (const role of ['Viewer', 'Editor'] as const) {
      const list = await accounts[role].get(MIDDLEWARES);
      expect(list.status()).toBe(200);
      expect((await create(accounts[role], middleware(TYPE, TYPE, sentinelConfig))).status()).toBe(403);
      const updated = await accounts[role].put(`${MIDDLEWARES}/${TYPE}`, {
        data: middleware(TYPE, TYPE, sentinelConfig),
      });
      await expectNoSentinel(updated);
      expect(updated.status()).toBe(403);
      expect((await accounts[role].delete(`${MIDDLEWARES}/${TYPE}`)).status()).toBe(403);
    }
  });

  test('Admin writes reach admission, which refuses unregistered types and mismatched names', async ({ accounts }) => {
    await expectRefused(
      await create(accounts.Admin, middleware(TYPE, TYPE, sentinelConfig)),
      `spec.type "${TYPE}" is not a tenant extension of this Gateway`
    );
    await expectRefused(
      await create(accounts.Admin, middleware(`${TYPE}-other`, TYPE, sentinelConfig)),
      'metadata.name must equal spec.type'
    );
    expect((await accounts.Admin.get(`${MIDDLEWARES}/${TYPE}`)).status()).toBe(404);
    expect((await accounts.Admin.delete(`${MIDDLEWARES}/${TYPE}`)).status()).toBe(404);
  });
});
