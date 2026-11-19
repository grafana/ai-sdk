import type { APIRequestContext, Page, Request, Response } from '@playwright/test';
import { test, expect, BROWSER_ROLES } from './fixtures';
import { API_GROUP, API_VERSION, catalogPageURL, prefixRoute, ROUTES } from '../src/constants';

const CATALOGS = `/apis/${API_GROUP}/${API_VERSION}/namespaces/default/modelcatalogs`;
const SENTINEL = 'e2e-catalog-sentinel-key';
const ACCEPTED_HEALTH_KEY = 'e2e-health-accepted-key';
const SUCCESS_BADGES = /^(Key saved|Key accepted|Model listed)$/;
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

async function createCatalog(page: Page, name: string, key: string) {
  await page.getByRole('textbox', { name: /^Catalog name/ }).fill(name);
  await page.getByLabel(/^API key/).fill(key);
  const created = page.waitForResponse((response) => isCatalogWrite('POST')(response.request()));
  await page.getByRole('button', { name: 'Create catalog' }).click();
  const response = await created;
  expect(response.status(), await response.text()).toBe(201);
  await expect(page).toHaveURL(new RegExp(`${catalogPageURL(name)}$`));
  return response;
}

async function addModel(page: Page, provider: string, providerModel: string) {
  const card = page.getByRole('region', { name: provider });
  await card.getByLabel(`Add a model to ${provider}`).fill(providerModel);
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

  test('an Admin saves blocked IDs before enabling unlisted calls', async ({ gotoPage, page, accounts }) => {
    const name = uniqueName();
    const seed = catalog(name, 'e2e-health-accepted-key');
    const created = await accounts.Admin.post(CATALOGS, { data: { ...seed, spec: { ...seed.spec, models: {} } } });
    expect(created.status(), await created.text()).toBe(201);
    const stored = async () => {
      const response = await accounts.Admin.get(`${CATALOGS}/${name}`);
      expect(response.ok(), await response.text()).toBe(true);
      return response.json();
    };
    const invoke = (id: string) =>
      accounts.Editor.post(`${CATALOGS}/${name}/language-model`, {
        headers: {
          'ai-language-model-id': id,
          'ai-language-model-specification-version': '4',
          'ai-language-model-streaming': 'false',
        },
        data: { prompt: [{ role: 'user', content: [{ type: 'text', text: 'hi' }] }], maxOutputTokens: 16 },
      });

    await gotoPage(`/${ROUTES.EditCatalog}/${name}`);
    const provider = card(page, 'claude');
    const toggle = provider.getByRole('switch', { name: 'Serve every model' });
    const blockedID = provider.getByRole('combobox', { name: 'Add a blocked model ID' });
    const blocklist = provider.getByRole('list', { name: 'Blocked model IDs' });
    const addBlockedID = provider.getByRole('button', { name: 'Add blocked ID' });
    const addModelID = provider.getByRole('combobox', { name: 'Add a model to claude' });
    const editBlocked = provider.getByRole('button', { name: 'Edit blocked IDs' });
    await expect(toggle).not.toBeChecked();
    await expect(addModelID).toBeVisible();
    await expect(blockedID).toHaveCount(0);
    await editBlocked.click();
    await expect(addModelID).toHaveCount(0);
    await blockedID.fill('org/blocked');
    await addBlockedID.click();
    await expect(blocklist.getByText('org/blocked')).toBeVisible();
    const before = await stored();
    expect(before.spec.providers.claude).toEqual({ type: 'anthropic', modelBlocklist: ['org/blocked'] });

    const enable = page.waitForResponse((response) => isCatalogWrite('PUT', name)(response.request()));
    await toggle.press('Space');
    const saved = await enable;
    expect(saved.status(), await saved.text()).toBe(200);
    const request = saved.request().postDataJSON();
    expect(request.metadata.resourceVersion).toBe(before.metadata.resourceVersion);
    expect(request).not.toHaveProperty('secure');
    await expect(toggle).toBeChecked();
    await expect(provider.getByRole('button', { name: 'Save serving settings' })).toHaveCount(0);
    const enabled = await stored();
    expect(enabled.spec.providers.claude).toEqual({
      type: 'anthropic',
      serveAllModels: true,
      modelBlocklist: ['org/blocked'],
    });
    await expect(provider.getByText('No listed models yet.')).toBeVisible();

    const discovery = await accounts.Editor.get(`${CATALOGS}/${name}/config`);
    expect(discovery.status(), await discovery.text()).toBe(200);
    expect(await discovery.json()).toEqual({ models: [] });
    const unlisted = await invoke('claude/org/new');
    expect(unlisted.status(), await unlisted.text()).toBe(200);
    expect(await unlisted.text()).toContain('org/new');
    const blocked = await invoke('claude/org/blocked');
    expect(blocked.status(), await blocked.text()).toBe(404);
    expect((await blocked.json()).error.code).toBe('model_not_found');

    await toggle.press('Space');
    await expect(toggle).not.toBeChecked();
    await expect(provider.getByText('No models yet.')).toBeVisible();
    expect((await stored()).spec.providers.claude).toEqual({
      type: 'anthropic',
      serveAllModels: false,
      modelBlocklist: ['org/blocked'],
    });
    const disabled = await invoke('claude/org/new');
    expect(disabled.status(), await disabled.text()).toBe(404);
    expect((await disabled.json()).error.code).toBe('model_not_found');

    const listed = 'claude-sonnet-4-6';
    await addModel(page, 'claude', listed);
    const beforeRefusal = await stored();
    await editBlocked.click();
    await expect(addModelID).toHaveCount(0);
    await blockedID.click();
    await provider.getByRole('option', { name: listed }).click();
    await expect(blockedID).toHaveValue(listed);
    await addBlockedID.click();
    await expect(
      provider.getByText(/providers\.claude\.modelBlocklist\[1\] conflicts with a listed target/)
    ).toBeVisible();
    await expect(toggle).not.toBeChecked();
    await expect(blockedID).toHaveValue(listed);
    await expect(blocklist.getByText('org/blocked')).toBeVisible();
    await expect(blocklist.getByText(listed)).toHaveCount(0);
    const afterRefusal = await stored();
    expect(afterRefusal.metadata.resourceVersion).toBe(beforeRefusal.metadata.resourceVersion);
    expect(afterRefusal.spec).toEqual(beforeRefusal.spec);
  });

  test('an Admin creates with a provider and can remove the last model but not that provider', async ({
    gotoPage,
    page,
    accounts,
  }) => {
    const name = uniqueName();
    const key = sentinelKey('create');
    await gotoPage(`/${ROUTES.Catalogs}`);
    await page.getByRole('link', { name: 'New catalog' }).click();
    await expect(page.getByRole('button', { name: 'Add model' })).toHaveCount(0);
    const watch = keyWatch(page);
    const created = await createCatalog(page, name, key);
    const createBody = created.request().postDataJSON();
    expect(createBody.spec).toEqual({ providers: { anthropic: { type: 'anthropic' } }, models: {} });
    expect(createBody.secure).toEqual({ anthropic: { create: key } });

    const provider = card(page, 'anthropic');
    await expect(provider.getByText('No models yet.')).toBeVisible();
    await expect(provider.getByText('Key rejected')).toBeVisible();
    const lastProvider = provider.getByRole('button', { name: 'Delete the catalog to remove its last provider.' });
    await expect(lastProvider).toBeDisabled();
    const model = 'claude-sonnet-4-6';
    await addModel(page, 'anthropic', model);

    const removedModel = page.waitForRequest(isCatalogWrite('PUT', name));
    await provider.getByRole('button', { name: `Remove ${model}` }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Remove' }).click();
    await expect(provider.getByText('No models yet.')).toBeVisible();
    expect((await removedModel).postDataJSON()).not.toHaveProperty('secure');
    const stored = await (await accounts.Admin.get(`${CATALOGS}/${name}`)).json();
    expect(stored.spec).toEqual(createBody.spec);
    expect(Object.keys(stored.secure.anthropic)).toEqual(['name']);
    await expect(lastProvider).toBeDisabled();
    await watch.nowhere(key);
  });

  test('an Admin edits a catalog one change at a time and keeps or replaces its key', async ({
    gotoPage,
    page,
    accounts,
  }) => {
    const { name, key, object } = await createStored(accounts.Admin);
    await gotoPage(`/${ROUTES.EditCatalog}/${name}`);
    await expect(card(page, 'claude')).toBeVisible();
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

    await page.reload();
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
    await dialog.getByRole('radio', { name: 'OpenAI', exact: true }).check();
    await dialog.getByLabel(/^API key/).fill(key);
    const added = page.waitForRequest(isCatalogWrite('PUT', name));
    await dialog.getByRole('button', { name: 'Connect' }).click();
    expect((await added).postDataJSON().secure).toEqual({ openai: { create: key } });
    await expect(card(page, 'openai')).toBeVisible();

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

  test('an admission refusal is shown as returned', async ({ gotoPage, page, accounts }) => {
    const { name, key } = await createStored(accounts.Admin);
    await gotoPage(`/${ROUTES.EditCatalog}/${name}`);
    await expect(card(page, 'claude').getByRole('button', { name: 'balanced', exact: true })).toBeVisible();
    const watch = keyWatch(page);
    await card(page, 'claude').getByRole('button', { name: 'balanced', exact: true }).click();
    const drawer = page.getByRole('dialog');
    const alias = drawer.getByPlaceholder('Add an alias and press Enter');
    await alias.fill('balanced');
    await alias.press('Enter');

    const refused = page.waitForResponse((response) => isCatalogWrite('PUT', name)(response.request()));
    await drawer.getByRole('button', { name: 'Save model' }).click();
    const response = await refused;
    expect(response.status()).toBe(400);
    const { message } = await response.json();
    expect(message).toContain(ADMISSION_REFUSAL);
    expect(message).toContain('alias "balanced" collides');
    await expect(drawer.getByRole('alert').filter({ hasText: message })).toBeVisible();
    expect(
      (await (await accounts.Admin.get(`${CATALOGS}/${name}`)).json()).spec.models.balanced.aliases
    ).toBeUndefined();
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

  test('an Admin checks the stored keys and models without exposing provider errors', async ({
    gotoPage,
    page,
    accounts,
  }) => {
    const name = uniqueName();
    const rejected = 'e2e-health-rejected-key';
    const object = catalog(name, ACCEPTED_HEALTH_KEY);
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
    await expect(card(page, 'claude').getByText('Model not listed')).toBeVisible();
    await expect(card(page, 'other').getByText('Key rejected')).toBeVisible();
    await expect(card(page, 'other').getByText('Not checked')).toBeVisible();
    await expect(page.getByRole('region', { name: 'Providers' }).getByRole('status')).toHaveText('3 problems');
    await expect(page.getByText(SUCCESS_BADGES)).toHaveCount(0);
    await expect(page.getByText('No problems found')).toHaveCount(0);
    await watch.nowhere(ACCEPTED_HEALTH_KEY);
    await watch.nowhere(rejected);
    await watch.nowhere('e2e-health-provider-error');
  });

  test('an all-OK catalog check shows one confirmation and no health badges', async ({ gotoPage, page, accounts }) => {
    const { name } = await createStored(accounts.Admin, ACCEPTED_HEALTH_KEY);
    await gotoPage(`/${ROUTES.EditCatalog}/${name}`);
    const check = page.getByRole('button', { name: 'Check health' });
    await expect(check).toBeVisible();
    await expect(page.getByText('No problems found')).toHaveCount(0);
    const finished = page.waitForResponse((result) => isCatalogWrite('POST', `${name}/health`)(result.request()));
    await check.click();
    expect((await finished).status()).toBe(200);
    await expect(page.getByRole('region', { name: 'Providers' }).getByRole('status')).toHaveText('No problems found');
    await expect(page.getByText(SUCCESS_BADGES)).toHaveCount(0);
  });

  test('an Admin checks and browses a provider before adding its first model', async ({ gotoPage, page }) => {
    const name = uniqueName();
    const accepted = 'e2e-health-accepted-key';
    const rejected = 'e2e-health-rejected-key';
    const listingRequests: Request[] = [];
    page.on('request', (request) => {
      if (isCatalogWrite('POST', `${name}/available-models`)(request)) {
        listingRequests.push(request);
      }
    });
    await gotoPage(`/${ROUTES.Catalogs}`);
    await page.getByRole('link', { name: 'New catalog' }).click();
    await expect(page.getByRole('textbox', { name: /^Catalog name/ })).toBeVisible();
    const watch = keyWatch(page);
    await expect(page.getByRole('combobox', { name: 'Add a model to anthropic' })).toHaveCount(0);
    const healthRequests: Request[] = [];
    page.on('request', (request) => {
      if (isCatalogWrite('POST', `${name}/health`)(request)) {
        healthRequests.push(request);
      }
    });
    const health = page.waitForResponse((response) => isCatalogWrite('POST', `${name}/health`)(response.request()));
    await createCatalog(page, name, accepted);

    const provider = card(page, 'anthropic');
    await expect(provider.getByText('No models yet.')).toBeVisible();
    expect(await (await health).json()).toEqual({ providers: { anthropic: 'ok' }, models: {} });
    await expect(page.getByRole('region', { name: 'Providers' }).getByRole('status')).toHaveText('No problems found');
    await expect(provider.getByText(SUCCESS_BADGES)).toHaveCount(0);
    const field = provider.getByRole('combobox', { name: 'Add a model to anthropic' });
    await expect(field).toBeVisible();
    await field.focus();
    expect(listingRequests).toHaveLength(0);
    await field.click();
    await field.fill('sonnet');
    await provider.getByRole('option', { name: 'claude-sonnet-4-6' }).click();
    await expect(field).toHaveValue('claude-sonnet-4-6');
    await provider.getByRole('button', { name: 'Add model' }).click();
    await expect(provider.getByRole('listitem', { name: 'claude-sonnet-4-6' })).toBeVisible();
    await field.fill('manual-after-listing');
    await expect(provider.getByText(/No matching models/)).toBeVisible();
    await field.press('Tab');
    await expect(field).toHaveValue('manual-after-listing');
    await provider.getByRole('button', { name: 'Add model' }).click();
    await expect(provider.getByRole('listitem', { name: 'manual-after-listing' })).toBeVisible();

    await provider.getByRole('button', { name: 'Replace key' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel(/^New API key/).fill(rejected);
    await dialog.getByRole('button', { name: 'Save key' }).click();
    await expect(dialog).toHaveCount(0);
    await field.click();
    await expect(provider.getByRole('status')).toHaveText('The provider rejected the saved key.');
    await expect(provider.getByRole('option')).toHaveCount(0);
    await addModel(page, 'anthropic', 'manual-with-rejected-key');

    expect(listingRequests).toHaveLength(3);
    for (const request of listingRequests) {
      expect(new URL(request.url()).search).toBe('?provider=anthropic');
      expect(request.postData()).toBeNull();
    }
    await watch.nowhere(accepted);
    await watch.nowhere(rejected);
    await watch.nowhere('e2e-health-provider-error');
    expect(healthRequests).toHaveLength(1);

    await page.reload();
    await expect(provider.getByRole('listitem', { name: 'manual-with-rejected-key' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Check health' })).toBeVisible();
    expect(healthRequests).toHaveLength(1);
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
      await expect(card(page, 'claude')).toBeVisible();
      await expect(page.getByText('Read-only')).toBeVisible();
      await expect(card(page, 'claude').getByText('Serves listed models only')).toBeVisible();
      await expect(card(page, 'claude').getByRole('switch', { name: 'Serve every model' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Connect a provider' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Replace key' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Delete catalog' })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Check health' })).toHaveCount(0);
      await expect(page.getByRole('combobox', { name: 'Add a model to claude' })).toHaveCount(0);
      const listingDenied = await page.request.post(`${CATALOGS}/${name}/available-models?provider=claude`);
      expect(listingDenied.status(), await listingDenied.text()).toBe(403);
      expect(await listingDenied.json()).toEqual(
        expect.objectContaining({
          status: 'Failure',
          reason: 'Forbidden',
          code: 403,
          message: expect.stringContaining('get modelcatalogs is not allowed'),
        })
      );
      const denied = await page.request.post(`${CATALOGS}/${name}/health`);
      expect(denied.status(), await denied.text()).toBe(403);
      expect(await denied.json()).toEqual(
        expect.objectContaining({
          status: 'Failure',
          reason: 'Forbidden',
          code: 403,
          message: expect.stringContaining('get modelcatalogs is not allowed'),
        })
      );

      await page.goto(prefixRoute(ROUTES.NewCatalog));
      await expect(page.getByText("You can't create model catalogs")).toBeVisible({ timeout: GRAFANA_BOOT_TIMEOUT_MS });
      await expect(page.getByRole('button', { name: 'Create catalog' })).toHaveCount(0);
    });
  }
});
