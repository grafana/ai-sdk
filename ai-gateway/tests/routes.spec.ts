import type { APIRequestContext } from '@playwright/test';
import { test, expect } from './fixtures';
import { API_GROUP, API_VERSION } from '../src/constants';

const PREFIX = `/apis/${API_GROUP}/${API_VERSION}/namespaces/default`;
const FAKE_MODEL = 'e2e/fake';
const FAKE_TEXT = 'fake stream';
const PROMPT = { prompt: [{ role: 'user', content: [{ type: 'text', text: 'hi' }] }], maxOutputTokens: 16 };

function callModel(request: APIRequestContext, modelID: string, streaming: boolean) {
  return request.post(`${PREFIX}/language-model`, {
    headers: {
      'ai-language-model-specification-version': '4',
      'ai-language-model-id': modelID,
      'ai-language-model-streaming': String(streaming),
    },
    data: PROMPT,
  });
}

function streamParts(body: string): Array<{ type: string; delta?: string }> {
  return body
    .split('\n')
    .filter((line) => line.startsWith('data: '))
    .map((line) => JSON.parse(line.slice('data: '.length)));
}

test.describe('namespace routes', () => {
  test('config lists the configured models', async ({ page }) => {
    const response = await page.request.get(`${PREFIX}/config`);
    expect(response.status()).toBe(200);
    expect(response.headers()['content-type']).toContain('application/json');
    const body = await response.text();
    expect(body).toContain('anthropic/claude-sonnet-5');
    expect(body).toContain(FAKE_MODEL);
  });

  test('language-model answers a unary call', async ({ page }) => {
    const response = await callModel(page.request, FAKE_MODEL, false);
    expect(response.status()).toBe(200);
    expect(response.headers()['content-type']).toContain('application/json');
    const body = await response.json();
    expect(body.content).toEqual([expect.objectContaining({ type: 'text', text: FAKE_TEXT })]);
  });

  test('language-model streams a call', async ({ page }) => {
    const response = await callModel(page.request, FAKE_MODEL, true);
    expect(response.status()).toBe(200);
    expect(response.headers()['content-type']).toContain('text/event-stream');
    const parts = streamParts(await response.text());
    const text = parts
      .filter((part) => part.type === 'text-delta')
      .map((part) => part.delta)
      .join('');
    expect(text).toBe(FAKE_TEXT);
    expect(parts.at(-1)?.type).toBe('finish');
  });

  test('language-model returns the Gateway error document for an unknown model', async ({ page }) => {
    const response = await callModel(page.request, 'e2e/missing', false);
    expect(response.status()).toBe(404);
    expect(response.headers()['content-type']).toContain('application/json');
    const body = await response.json();
    expect(body.error).toEqual(expect.objectContaining({ code: 'model_not_found', statusCode: 404 }));
  });

  test('undeclared methods are refused before the Gateway', async ({ page }) => {
    const config = await page.request.post(`${PREFIX}/config`, { data: {} });
    expect(config.status()).toBe(405);
    const language = await page.request.get(`${PREFIX}/language-model`);
    expect(language.status()).toBe(405);
  });
});
