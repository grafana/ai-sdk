import { test, expect } from './fixtures';
import { API_GROUP, API_VERSION } from '../src/constants';

const EXAMPLE_MODEL = 'anthropic/claude-sonnet-5';

test('serves the configured catalog from the plugin backend', async ({ page }) => {
  const response = await page.request.get(`/apis/${API_GROUP}/${API_VERSION}/namespaces/default/config`);
  expect(response.status()).toBe(200);
  expect(await response.text()).toContain(EXAMPLE_MODEL);
});
