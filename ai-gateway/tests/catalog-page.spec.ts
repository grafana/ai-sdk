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

async function fillNewCatalog(page: Page, name: string, key: string) {
  await page.getByRole('textbox', { name: /^Catalog name/ }).fill(name);
  await page.getByRole('button', { name: 'Connect a provider' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel(/^API key/).fill(key);
  await dialog.getByRole('button', { name: 'Connect' }).click();
  await expect(dialog).toHaveCount(0);
  await addModel(page, 'anthropic', 'claude-sonnet-4-6');
}

async function addModel(page: Page, provider: string, providerModel: string) {
  const card = page.getByRole('region', { name: provider });
  await card.getByRole('textbox', { name: `Add a model to ${provider}` }).fill(providerModel);
  await card.getByRole('button', { name: 'Add model' }).click();
  await expect(card.getByRole('listitem', { name: providerModel, exact: true })).toBeVisible();
}

function card(page: Page, provider: string) {
  return page.getByRole('region', { name: provider, exact: true });
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
    await page.getByRole('link', { name: 'New catalog' }).click();
    await expect(page.getByRole('textbox', { name: /^Catalog name/ })).toBeVisible();
    const watch = keyWatch(page);
    await fillNewCatalog(page, name, key);

    const created = page.waitForRequest(isCatalogWrite('POST'));
    await page.getByRole('button', { name: 'Create catalog' }).click();
    expect((await created).postDataJSON().secure).toEqual({ anthropic: { create: key } });
    await expect(page).toHaveURL(new RegExp(`${catalogPageURL(name)}$`));
    await expect(card(page, 'anthropic').getByText('Key saved')).toBeVisible();
    await expect(card(page, 'anthropic').getByRole('listitem', { name: 'claude-sonnet-4-6' })).toBeVisible();

    const stored = await (await accounts.Admin.get(`${CATALOGS}/${name}`)).json();
    expect(Object.keys(stored.secure.anthropic)).toEqual(['name']);
    await watch.nowhere(key);
  });

  test('an Admin edits a catalog one change at a time and keeps or replaces its key', async ({
    gotoPage,
    page,
    accounts,
  }) => {
    const { name, key, object } = await createStored(accounts.Admin);
    await gotoPage(`/${ROUTES.EditCatalog}/${name}`);
    await expect(card(page, 'claude').getByText('Key saved')).toBeVisible();
    const watch = keyWatch(page);

    await card(page, 'claude').getByRole('button', { name: 'balanced', exact: true }).click();
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
    await expect(card(page, 'claude').getByText('Renamed')).toBeVisible();
    const afterKept = await (await accounts.Admin.get(`${CATALOGS}/${name}`)).json();
    expect(afterKept.spec.models.balanced.name).toBe('Renamed');
    expect(afterKept.secure).toEqual(object.secure);

    const added = page.waitForResponse((response) => isCatalogWrite('PUT', name)(response.request()));
    await addModel(page, 'claude', 'claude-haiku-4-5');
    const addedResponse = await added;
    expect(addedResponse.request().postDataJSON().metadata.resourceVersion).toBe(
      (await keptResponse.json()).metadata.resourceVersion
    );

    const replacement = sentinelKey('replaced');
    await card(page, 'claude').getByRole('button', { name: 'Replace key' }).click();
    await page
      .getByRole('dialog')
      .getByLabel(/^New API key/)
      .fill(replacement);
    const replaced = page.waitForRequest(isCatalogWrite('PUT', name));
    await page.getByRole('dialog').getByRole('button', { name: 'Save key' }).click();
    const replacedBody = (await replaced).postDataJSON();
    expect(replacedBody.secure).toEqual({ claude: { create: replacement } });
    expect(replacedBody.metadata.resourceVersion).toBe((await addedResponse.json()).metadata.resourceVersion);
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(card(page, 'claude').getByText('Key saved')).toBeVisible();

    await page.reload();
    await expect(card(page, 'claude').getByText('Key saved')).toBeVisible();
    await expect(card(page, 'claude').getByRole('listitem', { name: 'claude-haiku-4-5' })).toBeVisible();
    await watch.nowhere(key);
    await watch.nowhere(replacement);
  });

  test('an Admin connects a provider with its key and removes it again', async ({ gotoPage, page, accounts }) => {
    const { name } = await createStored(accounts.Admin);
    await gotoPage(`/${ROUTES.EditCatalog}/${name}`);
    await expect(card(page, 'claude')).toBeVisible();
    const watch = keyWatch(page);
    const key = sentinelKey('added');

    await page.getByRole('button', { name: 'Connect a provider' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('radio', { name: 'OpenAI' }).check();
    await dialog.getByLabel(/^API key/).fill(key);
    const added = page.waitForRequest(isCatalogWrite('PUT', name));
    await dialog.getByRole('button', { name: 'Connect' }).click();
    expect((await added).postDataJSON().secure).toEqual({ openai: { create: key } });
    await expect(card(page, 'openai').getByText('Key saved')).toBeVisible();

    await card(page, 'openai').getByRole('button', { name: 'Remove openai' }).click();
    const removed = page.waitForRequest(isCatalogWrite('PUT', name));
    await page.getByRole('dialog').getByRole('button', { name: 'Remove' }).click();
    expect((await removed).postDataJSON().secure).toEqual({ openai: { remove: true } });
    await expect(card(page, 'openai')).toHaveCount(0);
    const stored = await (await accounts.Admin.get(`${CATALOGS}/${name}`)).json();
    expect(Object.keys(stored.secure)).toEqual(['claude']);
    await watch.nowhere(key);
  });

  test('an Admin deletes a catalog after confirming', async ({ gotoPage, page, accounts }) => {
    const { name } = await createStored(accounts.Admin);
    await gotoPage(`/${ROUTES.Catalogs}`);
    await expect(page.getByRole('button', { name: /Delete/ })).toHaveCount(0);

    await page.getByRole('link', { name }).click();
    await page.getByRole('button', { name: 'Delete catalog' }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Delete' }).click();
    await expect(page).toHaveURL(new RegExp(`${prefixRoute(ROUTES.Catalogs)}$`));
    await expect(page.getByRole('link', { name })).toHaveCount(0);
    expect((await accounts.Admin.get(`${CATALOGS}/${name}`)).status()).toBe(404);
  });

  test('an admission refusal is shown as returned', async ({ gotoPage, page }) => {
    const key = sentinelKey('refused');
    await gotoPage(`/${ROUTES.NewCatalog}`);
    await expect(page.getByRole('textbox', { name: /^Catalog name/ })).toBeVisible();
    const watch = keyWatch(page);
    await fillNewCatalog(page, uniqueName(), key);
    await card(page, 'anthropic').getByRole('button', { name: 'claude-sonnet-4-6', exact: true }).click();
    const drawer = page.getByRole('dialog');
    await drawer.getByPlaceholder('Add an alias and press Enter').fill('claude-sonnet-4-6');
    await drawer.getByPlaceholder('Add an alias and press Enter').press('Enter');
    await drawer.getByRole('button', { name: 'Save model' }).click();
    await expect(drawer).toHaveCount(0);

    const refused = page.waitForResponse((response) => isCatalogWrite('POST')(response.request()));
    await page.getByRole('button', { name: 'Create catalog' }).click();
    const response = await refused;
    expect(response.status()).toBe(400);
    const { message } = await response.json();
    expect(message).toContain(ADMISSION_REFUSAL);
    expect(message).toContain('alias "claude-sonnet-4-6" collides');
    await expect(page.getByRole('alert').filter({ hasText: message })).toBeVisible();
    await expect(card(page, 'anthropic').getByText('Also requested as claude-sonnet-4-6')).toBeVisible();
    expect(await page.locator('body').innerText()).not.toContain(key);
    await watch.notFetched(key);
    await page.getByRole('link', { name: 'Cancel' }).click();
    await expect(page.getByRole('textbox', { name: /^Catalog name/ })).toHaveCount(0);
    await watch.nowhere(key);
  });

  test('a concurrent edit is refused as returned', async ({ gotoPage, page, accounts }) => {
    const { name, object } = await createStored(accounts.Admin);
    await gotoPage(`/${ROUTES.EditCatalog}/${name}`);
    await expect(card(page, 'claude')).toBeVisible();

    const changed = {
      ...object,
      spec: {
        ...object.spec,
        models: { other: { name: 'Other', primary: { provider: 'claude', model: 'claude-haiku' } } },
      },
    };
    expect((await accounts.Admin.put(`${CATALOGS}/${name}`, { data: changed })).status()).toBe(200);

    await card(page, 'claude').getByRole('button', { name: 'balanced', exact: true }).click();
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

  test('an Admin checks the stored keys and models without exposing provider errors', async ({ gotoPage, page, accounts }) => {
    const name = uniqueName();
    const accepted = 'e2e-health-accepted-key';
    const rejected = 'e2e-health-rejected-key';
    const object = catalog(name, accepted);
    const response = await accounts.Admin.post(CATALOGS, {
      data: {
        ...object,
        spec: {
          providers: { ...object.spec.providers, other: { type: 'anthropic' } },
          models: {
            ...object.spec.models,
            typo: { name: 'Typo', primary: { provider: 'claude', model: 'not-a-model' } },
            rejected: { name: 'Rejected', primary: { provider: 'other', model: 'claude-sonnet-4-6' } },
          },
        },
        secure: { ...object.secure, other: { create: rejected } },
      },
    });
    expect(response.status(), await response.text()).toBe(201);
    await gotoPage(`/${ROUTES.EditCatalog}/${name}`);
    const check = page.getByRole('button', { name: 'Check health' });
    await expect(check).toBeVisible();
    const watch = keyWatch(page);
    const finished = page.waitForResponse((result) => isCatalogWrite('POST', `${name}/health`)(result.request()));
    await check.click();
    expect((await finished).status()).toBe(200);
    await expect(card(page, 'claude').getByText('Listing accepted')).toBeVisible();
    await expect(card(page, 'claude').getByText('Model listed', { exact: true })).toBeVisible();
    await expect(card(page, 'claude').getByText('Model not listed')).toBeVisible();
    await expect(card(page, 'other').getByText('Key rejected')).toBeVisible();
    await expect(card(page, 'other').getByText('Not checked')).toBeVisible();
    await watch.nowhere(accepted);
    await watch.nowhere(rejected);
    await watch.nowhere('e2e-health-provider-error');
  });

  for (const role of BROWSER_ROLES) {
    test(`${role} users see catalogs read-only`, async ({ pageAs, accounts }) => {
      test.slow();
      const { name } = await createStored(accounts.Admin);
      const page = await pageAs(role);

      await page.goto(prefixRoute(ROUTES.Catalogs));
      await expect(page.getByRole('link', { name })).toBeVisible({ timeout: GRAFANA_BOOT_TIMEOUT_MS });
      await expect(page.getByRole('link', { name: 'New catalog' })).toHaveCount(0);

      await page.getByRole('link', { name }).click();
      await expect(card(page, 'claude').getByText('Key saved')).toBeVisible();
      await expect(page.getByText('Read-only')).toBeVisible();
      await expect(page.getByRole('button', { name: 'Connect a provider' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Replace key' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Delete catalog' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Check health' })).toHaveCount(0);
      expect((await accounts[role].post(`${CATALOGS}/${name}/health`)).status()).toBe(403);

      await page.goto(prefixRoute(ROUTES.NewCatalog));
      await expect(page.getByText("You can't create model catalogs")).toBeVisible({ timeout: GRAFANA_BOOT_TIMEOUT_MS });
      await expect(page.getByRole('button', { name: 'Create catalog' })).toHaveCount(0);
    });
  }
});
