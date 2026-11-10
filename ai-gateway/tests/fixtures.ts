import type { APIRequestContext } from '@playwright/test';
import { AppPage, expect, test as base } from '@grafana/plugin-e2e';
import pluginJson from '../src/plugin.json';

export const ROLES = ['Viewer', 'Editor', 'Admin'] as const;
export type Role = (typeof ROLES)[number];

const TOKEN_SECONDS = 3600;

type AppTestFixture = {
  gotoPage: (path?: string) => Promise<AppPage>;
};

type WorkerFixture = {
  // accounts are API clients authenticated as one service account per basic
  // role, created as the Admin and deleted when the worker ends.
  accounts: Record<Role, APIRequestContext>;
};

export const test = base.extend<AppTestFixture, WorkerFixture>({
  gotoPage: async ({ gotoAppPage }, use) => {
    await use((path) =>
      gotoAppPage({
        path,
        pluginId: pluginJson.id,
      })
    );
  },
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

export { expect };
