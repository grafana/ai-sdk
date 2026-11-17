import type { APIRequestContext, Page, Request, Response } from '@playwright/test';
import { test, expect, BROWSER_ROLES } from './fixtures';
import { API_GROUP, API_VERSION, catalogPageURL, prefixRoute, ROUTES } from '../src/constants';

const CATALOGS = `/apis/${API_GROUP}/${API_VERSION}/namespaces/default/modelcatalogs`;
const SENTINEL = 'e2e-catalog-sentinel-key';
const ADMISSION_REFUSAL = 'admission hook for ModelCatalog denied the request';
const GRAFANA_BOOT_TIMEOUT_MS = 20_000;

let sequence = 0;

function uniqueName(): string {
  sequence += 1;
  return `e2e-page-${process.pid}-${Date.now()}-${sequence}`;
}

function sentinelKey(label: string): string {
  return `${SENTINEL}-page-${label}-${process.pid}-${sequence}`;
}

function catalog(name: string, key: string) {
  return {
    apiVersion: `${API_GROUP}/${API_VERSION}`,
    kind: 'ModelCatalog',
    metadata: { name },
    spec: {
      providers: { claude: { type: 'anthropic' } },
      models: { balanced: { name: 'Balanced', primary: { provider: 'claude', model: 'claude-sonnet-4-6' } } },
    },
    secure: { claude: { create: key } },
  };
}

async function createStored(request: APIRequestContext, key = sentinelKey('stored')) {
  const name = uniqueName();
  const response = await request.post(CATALOGS, { data: catalog(name, key) });
  expect(response.status(), await response.text()).toBe(201);
  return { name, key, object: await response.json() };
}

// keyWatch records every browser console message and every response body the
// page fetches from the time it is called, except watch streams, which never
// end. Tests call it once Grafana has loaded, before a key is typed, because
// loading navigates and drops the bodies of Grafana's own boot requests.
// notFetched checks the console and the responses, and nowhere also checks the
// page.
function keyWatch(page: Page) {
  const messages: string[] = [];
  const bodies: Array<Promise<string>> = [];
  const unreadable: string[] = [];
  page.on('console', (message) => messages.push(message.text()));
  page.on('response', (response: Response) => {
    const request = response.request();
    const type = request.resourceType();
    const watch = new URL(request.url()).searchParams.get('watch') === 'true';
    if ((type === 'fetch' || type === 'xhr') && !watch && !request.redirectedTo()) {
      bodies.push(
        response.text().catch((error: Error) => {
          unreadable.push(`${response.url()}: ${error.message}`);
          return '';
        })
      );
    }
  });
  const notFetched = async (key: string) => {
    expect(messages.filter((message) => message.includes(key))).toEqual([]);
    for (const body of await Promise.all(bodies)) {
      expect(body).not.toContain(key);
    }
    expect(unreadable).toEqual([]);
  };
  const nowhere = async (key: string) => {
    await notFetched(key);
    expect(await page.content()).not.toContain(key);
  };
  return { notFetched, nowhere };
}

function isCatalogWrite(method: string, name?: string) {
  const path = name ? `${CATALOGS}/${name}` : CATALOGS;
  return (request: Request) => request.method() === method && new URL(request.url()).pathname.endsWith(path);
}

async function fillNewCatalog(page: Page, name: string, key: string, modelID = 'balanced') {
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('textbox', { name: /^Catalog name/ }).fill(name);
  await dialog.getByLabel(/^API key/).fill(key);
  await dialog.getByRole('textbox', { name: /^Model ID/ }).fill(modelID);
  await dialog.getByRole('textbox', { name: /^Display name/ }).fill('Balanced');
  await dialog.getByRole('textbox', { name: /^Provider model/ }).fill('claude-sonnet-4-6');
  return dialog;
}

function row(page: Page, name: string) {
  return page.getByRole('listitem', { name, exact: true });
}

test.describe('model catalog page', () => {
  test.afterEach(async ({ accounts }) => {
    const list = await accounts.Admin.get(CATALOGS);
    for (const item of (await list.json()).items ?? []) {
      if (item.metadata.name.startsWith(`e2e-page-${process.pid}-`)) {
        await accounts.Admin.delete(`${CATALOGS}/${item.metadata.name}`);
      }
    }
  });

  test('an Admin creates a catalog and its key is never shown', async ({ gotoPage, page, accounts }) => {
    const name = uniqueName();
    const key = sentinelKey('create');
    await gotoPage(`/${ROUTES.Catalogs}`);
    await page.getByRole('button', { name: 'New catalog' }).click();
    const watch = keyWatch(page);
    const dialog = await fillNewCatalog(page, name, key);

    const created = page.waitForRequest(isCatalogWrite('POST'));
    await dialog.getByRole('button', { name: 'Create catalog' }).click();
    expect((await created).postDataJSON().secure).toEqual({ anthropic: { create: key } });
    await expect(page).toHaveURL(new RegExp(`${catalogPageURL(name)}$`));
    await expect(row(page, 'anthropic').getByText('Key saved')).toBeVisible();
    await expect(row(page, 'balanced')).toBeVisible();

    const stored = await (await accounts.Admin.get(`${CATALOGS}/${name}`)).json();
    expect(Object.keys(stored.secure.anthropic)).toEqual(['name']);
    await watch.nowhere(key);
  });

  test('an Admin edits a model and replaces a key, each saved at once', async ({ gotoPage, page, accounts }) => {
    const { name, key, object } = await createStored(accounts.Admin);
    await gotoPage(`/${ROUTES.Catalogs}/${name}`);
    await expect(row(page, 'claude').getByText('Key saved')).toBeVisible();
    const watch = keyWatch(page);

    await page.getByRole('button', { name: 'Edit balanced' }).click();
    await page
      .getByRole('dialog')
      .getByRole('textbox', { name: /^Display name/ })
      .fill('Renamed');
    const kept = page.waitForResponse((response) => isCatalogWrite('PUT', name)(response.request()));
    await page.getByRole('dialog').getByRole('button', { name: 'Save model' }).click();
    const keptResponse = await kept;
    const keptBody = keptResponse.request().postDataJSON();
    expect(keptBody).not.toHaveProperty('secure');
    expect(keptBody.metadata.resourceVersion).toBe(object.metadata.resourceVersion);
    await expect(row(page, 'balanced').getByText('Renamed')).toBeVisible();
    const afterKept = await (await accounts.Admin.get(`${CATALOGS}/${name}`)).json();
    expect(afterKept.spec.models.balanced.name).toBe('Renamed');
    expect(afterKept.secure).toEqual(object.secure);

    const replacement = sentinelKey('replaced');
    await page.getByRole('button', { name: 'Edit claude' }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Reset' }).click();
    await page
      .getByRole('dialog')
      .getByLabel(/^API key/)
      .fill(replacement);
    const replaced = page.waitForRequest(isCatalogWrite('PUT', name));
    await page.getByRole('dialog').getByRole('button', { name: 'Save provider' }).click();
    const replacedBody = (await replaced).postDataJSON();
    expect(replacedBody.secure).toEqual({ claude: { create: replacement } });
    expect(replacedBody.metadata.resourceVersion).toBe((await keptResponse.json()).metadata.resourceVersion);
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(row(page, 'claude').getByText('Key saved')).toBeVisible();

    await page.reload();
    await expect(row(page, 'claude').getByText('Key saved')).toBeVisible();
    await watch.nowhere(key);
    await watch.nowhere(replacement);
  });

  test('an Admin adds a provider with its key and removes it again', async ({ gotoPage, page, accounts }) => {
    const { name } = await createStored(accounts.Admin);
    await gotoPage(`/${ROUTES.Catalogs}/${name}`);
    await expect(row(page, 'claude')).toBeVisible();
    const watch = keyWatch(page);
    const key = sentinelKey('added');

    await page.getByRole('button', { name: 'Add provider' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByText('OpenAI', { exact: true }).click();
    await dialog.getByLabel(/^API key/).fill(key);
    const added = page.waitForRequest(isCatalogWrite('PUT', name));
    await dialog.getByRole('button', { name: 'Add provider' }).click();
    expect((await added).postDataJSON().secure).toEqual({ openai: { create: key } });
    await expect(row(page, 'openai').getByText('Key saved')).toBeVisible();

    await row(page, 'openai').getByRole('button', { name: 'Remove openai' }).click();
    const removed = page.waitForRequest(isCatalogWrite('PUT', name));
    await page.getByRole('dialog').getByRole('button', { name: 'Remove' }).click();
    expect((await removed).postDataJSON().secure).toEqual({ openai: { remove: true } });
    await expect(row(page, 'openai')).toHaveCount(0);
    const stored = await (await accounts.Admin.get(`${CATALOGS}/${name}`)).json();
    expect(Object.keys(stored.secure)).toEqual(['claude']);
    await watch.nowhere(key);
  });

  test('an Admin deletes a catalog after confirming', async ({ gotoPage, page, accounts }) => {
    const { name } = await createStored(accounts.Admin);
    await gotoPage(`/${ROUTES.Catalogs}`);

    await page.getByRole('button', { name: `Delete ${name}` }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Delete' }).click();
    await expect(page.getByRole('link', { name })).toHaveCount(0);
    expect((await accounts.Admin.get(`${CATALOGS}/${name}`)).status()).toBe(404);
  });

  test('an admission refusal is shown as returned', async ({ gotoPage, page }) => {
    const key = sentinelKey('refused');
    await gotoPage(`/${ROUTES.Catalogs}`);
    await page.getByRole('button', { name: 'New catalog' }).click();
    const watch = keyWatch(page);
    const dialog = await fillNewCatalog(page, uniqueName(), key, 'bad id');

    const refused = page.waitForResponse((response) => isCatalogWrite('POST')(response.request()));
    await dialog.getByRole('button', { name: 'Create catalog' }).click();
    const response = await refused;
    expect(response.status()).toBe(400);
    const { message } = await response.json();
    expect(message).toContain(ADMISSION_REFUSAL);
    expect(message).toContain('public ID');
    await expect(dialog.getByRole('alert').filter({ hasText: message })).toBeVisible();
    await expect(dialog.getByRole('textbox', { name: /^Model ID/ })).toHaveValue('bad id');
    expect(await page.locator('body').innerText()).not.toContain(key);
    await watch.notFetched(key);
    await dialog.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await watch.nowhere(key);
  });

  test('a concurrent edit is refused as returned', async ({ gotoPage, page, accounts }) => {
    const { name, object } = await createStored(accounts.Admin);
    await gotoPage(`/${ROUTES.Catalogs}/${name}`);
    await expect(row(page, 'balanced')).toBeVisible();

    const changed = {
      ...object,
      spec: {
        ...object.spec,
        models: { other: { name: 'Other', primary: { provider: 'claude', model: 'claude-haiku' } } },
      },
    };
    expect((await accounts.Admin.put(`${CATALOGS}/${name}`, { data: changed })).status()).toBe(200);

    await page.getByRole('button', { name: 'Edit balanced' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('textbox', { name: /^Display name/ }).fill('Stale');
    const conflict = page.waitForResponse((response) => isCatalogWrite('PUT', name)(response.request()));
    await dialog.getByRole('button', { name: 'Save model' }).click();
    const response = await conflict;
    expect(response.status()).toBe(409);
    const { message } = await response.json();
    await expect(dialog.getByRole('alert').filter({ hasText: message })).toBeVisible();
    const current = await (await accounts.Admin.get(`${CATALOGS}/${name}`)).json();
    expect(Object.keys(current.spec.models)).toEqual(['other']);
  });

  for (const role of BROWSER_ROLES) {
    test(`${role} users see catalogs read-only`, async ({ pageAs, accounts }) => {
      test.slow();
      const { name } = await createStored(accounts.Admin);
      const page = await pageAs(role);

      await page.goto(prefixRoute(ROUTES.Catalogs));
      await expect(page.getByRole('link', { name })).toBeVisible({ timeout: GRAFANA_BOOT_TIMEOUT_MS });
      await expect(page.getByRole('button', { name: 'New catalog' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: `Delete ${name}` })).toHaveCount(0);

      await page.getByRole('link', { name }).click();
      await expect(row(page, 'claude').getByText('Key saved')).toBeVisible();
      await expect(page.getByText('Read-only')).toBeVisible();
      await expect(page.getByRole('button', { name: 'Add provider' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Add model' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Edit balanced' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Delete catalog' })).toHaveCount(0);
    });
  }
});
