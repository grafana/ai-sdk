import { test, expect } from './fixtures';
import { API_GROUP, API_VERSION, ROUTES } from '../src/constants';

const CATALOGS = `/apis/${API_GROUP}/${API_VERSION}/namespaces/default/modelcatalogs`;

test.describe('overview page', () => {
  test('shows the Gateway endpoint for this stack', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Overview}`);
    await expect(page.getByRole('heading', { name: 'Quick start' })).toBeVisible();
    await expect(page.getByText(/\/apis\/aigateway\.ext\.grafana\.app\/v1alpha1\/namespaces\//).first()).toBeVisible();
  });

  test('opens from the app root', async ({ gotoPage, page }) => {
    await gotoPage('/');
    await expect(page.getByRole('heading', { name: 'Endpoint' })).toBeVisible();
  });

  test('shows the base URL of each model catalog', async ({ gotoPage, page, accounts }) => {
    const name = `e2e-overview-${process.pid}-${Date.now()}`;
    const created = await accounts.Admin.post(CATALOGS, {
      data: {
        apiVersion: `${API_GROUP}/${API_VERSION}`,
        kind: 'ModelCatalog',
        metadata: { name },
        spec: {
          providers: { claude: { type: 'anthropic' } },
          models: { balanced: { name: 'Balanced', primary: { provider: 'claude', model: 'claude-sonnet-4-6' } } },
        },
        secure: { claude: { create: 'e2e-catalog-sentinel-key-overview' } },
      },
    });
    expect(created.status(), await created.text()).toBe(201);
    try {
      await gotoPage(`/${ROUTES.Overview}`);
      await expect(page.getByRole('heading', { name: 'Model catalogs' })).toBeVisible();
      const baseURL = new RegExp(`^https?://.+${CATALOGS.replaceAll('.', '\\.')}/${name}$`);
      await expect(page.getByText(baseURL)).toBeVisible();
    } finally {
      await accounts.Admin.delete(`${CATALOGS}/${name}`);
    }
  });
});
