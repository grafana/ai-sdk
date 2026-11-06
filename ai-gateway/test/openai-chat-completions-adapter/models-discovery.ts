import assert from 'node:assert/strict';
import OpenAI from 'openai';

// Run against the production namespace routes before any generation call.
export async function verifyModelDiscovery(client: OpenAI): Promise<void> {
  const page = await client.models.list();
  const ids = page.data.map((model) => model.id);
  assert.deepEqual(ids, [...new Set(ids)].sort());
  assert.ok(ids.includes('chat'));
  assert.ok(ids.includes('public/chat'));
  for (const id of ['chat', 'public/chat']) {
    const model = await client.models.retrieve(id);
    assert.deepEqual(model, { id, object: 'model', created: 0, owned_by: 'grafana' });
  }
  await assert.rejects(client.models.retrieve('private/unknown'), (error: unknown) => {
    assert.ok(error instanceof OpenAI.APIError);
    assert.equal(error.status, 404);
    assert.equal(error.code, 'model_not_found');
    return true;
  });
}
