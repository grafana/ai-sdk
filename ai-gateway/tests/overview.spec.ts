import { test, expect } from './fixtures';
import { ROUTES } from '../src/constants';

test.describe('overview page', () => {
  test('shows the Grafana Cloud quick start with this stack endpoint', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Overview}`);
    await expect(page.getByRole('heading', { name: 'Get started in Grafana Cloud' })).toBeVisible();
    await expect(page.getByText(/\/apis\/aigateway\.ext\.grafana\.app\/v1alpha1\/namespaces\//).first()).toBeVisible();
    await expect(page.getByText(/providerOptions: \{/)).toBeVisible();
  });

  test('opens from the app root', async ({ gotoPage, page }) => {
    await gotoPage('/');
    await expect(page.getByRole('heading', { name: 'Choose how you call' })).toBeVisible();
  });

  test('expands self-hosted guidance', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Overview}`);
    await page.getByText('Running Grafana yourself?').click();
    await expect(page.getByRole('link', { name: /Configure models/ })).toBeVisible();
  });
});
