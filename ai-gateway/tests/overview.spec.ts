import { test, expect } from './fixtures';
import { ROUTES } from '../src/constants';

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
});
