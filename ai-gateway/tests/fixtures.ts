import { randomUUID } from 'node:crypto';
import type { APIRequestContext, BrowserContext, Page } from '@playwright/test';
import { AppPage, expect, test as base } from '@grafana/plugin-e2e';
import pluginJson from '../src/plugin.json';

export const ROLES = ['Viewer', 'Editor', 'Admin'] as const;
export type Role = (typeof ROLES)[number];

export const BROWSER_ROLES = ['Viewer', 'Editor'] as const;
export type BrowserRole = (typeof BROWSER_ROLES)[number];

const TOKEN_SECONDS = 3600;
const ADMIN_STATE = 'playwright/.auth/admin.json';

type Credentials = { user: string; password: string };

type AppTestFixture = {
  gotoPage: (path?: string) => Promise<AppPage>;
  // pageAs opens a page in a new browser context signed in as the worker's
  // login user with that basic role. The contexts close after the test.
  pageAs: (role: BrowserRole) => Promise<Page>;
};

type WorkerFixture = {
  // accounts are API clients authenticated as one service account per basic
  // role, created as the Admin and deleted when the worker ends.
  accounts: Record<Role, APIRequestContext>;
  // users are login users with the Viewer and Editor basic roles, created as
  // the Admin and deleted when the worker ends.
  users: Record<BrowserRole, Credentials>;
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
  pageAs: async ({ browser, baseURL, users }, use) => {
    const contexts: BrowserContext[] = [];
    await use(async (role) => {
      const context = await browser.newContext({ baseURL, storageState: { cookies: [], origins: [] } });
      contexts.push(context);
      const login = await context.request.post('/login', { data: users[role] });
      expect(login.status(), await login.text()).toBe(200);
      return context.newPage();
    });
    for (const context of contexts) {
      await context.close();
    }
  },
  users: [
    async ({ playwright }, use, workerInfo) => {
      const admin = await playwright.request.newContext({
        baseURL: workerInfo.project.use.baseURL,
        storageState: ADMIN_STATE,
      });
      const users = {} as Record<BrowserRole, Credentials>;
      const ids: number[] = [];
      for (const role of BROWSER_ROLES) {
        const user = `e2e-user-${role.toLowerCase()}-${workerInfo.workerIndex}-${Date.now()}`;
        const password = randomUUID();
        const created = await admin.post('/api/admin/users', { data: { name: user, login: user, password } });
        expect(created.status(), await created.text()).toBe(200);
        const { id } = await created.json();
        ids.push(id);
        const updated = await admin.patch(`/api/org/users/${id}`, { data: { role } });
        expect(updated.status(), await updated.text()).toBe(200);
        users[role] = { user, password };
      }
      await use(users);
      for (const id of ids) {
        await admin.delete(`/api/admin/users/${id}`);
      }
      await admin.dispose();
    },
    { scope: 'worker' },
  ],
  accounts: [
    async ({ playwright }, use, workerInfo) => {
      const baseURL = workerInfo.project.use.baseURL;
      const admin = await playwright.request.newContext({ baseURL, storageState: ADMIN_STATE });
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
