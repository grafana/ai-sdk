import type { APIRequestContext } from '@playwright/test';
import { test as base, expect } from './fixtures';

export const ROLES = ['Viewer', 'Editor', 'Admin'] as const;
const TOKEN_SECONDS = 3600;

export type Role = (typeof ROLES)[number];

export const test = base.extend<object, { accounts: Record<Role, APIRequestContext> }>({
  trace: ['off', { scope: 'worker' }],
  accounts: [
    async ({ playwright }, use, workerInfo) => {
      const baseURL = workerInfo.project.use.baseURL;
      const admin = await playwright.request.newContext({ baseURL, storageState: 'playwright/.auth/admin.json' });
      const accounts = {} as Record<Role, APIRequestContext>;
      const ids: number[] = [];
      for (const role of ROLES) {
        const name = `e2e-${role.toLowerCase()}-${workerInfo.workerIndex}-${Date.now()}`;
        const account = await admin.post('/api/serviceaccounts', { data: { name, role } });
        expect(account.status(), await account.text()).toBe(201);
        const { id } = await account.json();
        ids.push(id);
        const token = await admin.post(`/api/serviceaccounts/${id}/tokens`, {
          data: { name, secondsToLive: TOKEN_SECONDS },
        });
        expect(token.status(), await token.text()).toBe(200);
        const { key } = await token.json();
        accounts[role] = await playwright.request.newContext({
          baseURL,
          extraHTTPHeaders: { Authorization: `Bearer ${key}` },
        });
      }
      await use(accounts);
      for (const role of ROLES) {
        await accounts[role].dispose();
      }
      for (const id of ids) {
        await admin.delete(`/api/serviceaccounts/${id}`);
      }
      await admin.dispose();
    },
    { scope: 'worker' },
  ],
});

export async function permissions(request: APIRequestContext): Promise<string[]> {
  return Object.keys(await (await request.get('/api/access-control/user/permissions?reloadcache=true')).json());
}

export { expect };
