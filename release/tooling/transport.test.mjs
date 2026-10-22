import assert from 'node:assert/strict';
import { once } from 'node:events';
import test from 'node:test';
import { Worker } from 'node:worker_threads';
import { readOnlyFetch } from './preview.mjs';
import { downloadArtifact } from './authorization-evidence.mjs';

for (const artifact of [false, true]) test(`reads survive synchronous validation after ${artifact ? 'artifact download' : 'API read'}`, async t => {
  const closed = new Int32Array(new SharedArrayBuffer(4));
  const server = new Worker(`
    const { parentPort, workerData } = require('node:worker_threads');
    const { createServer } = require('node:http');
    let connections = 0;
    const server = createServer((req, res) => {
      res.setHeader('X-Request-Connection', req.headers.connection);
      if (req.url === '/artifact') {
        res.writeHead(302, { location: 'https://example.blob.core.windows.net/storage' });
        res.end();
        return;
      }
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({ connection: req.socket.connectionId, auth: req.headers.authorization }));
    });
    server.on('connection', socket => { socket.connectionId = ++connections; });
    server.listen(0, '127.0.0.1', () => parentPort.postMessage(server.address().port));
    parentPort.on('message', () => {
      server.closeIdleConnections();
      Atomics.store(workerData, 0, 1);
      Atomics.notify(workerData, 0);
    });
  `, { eval: true, workerData: closed });
  t.after(() => server.terminate());
  const [port] = await once(server, 'message');
  const localFetch = async (url, options) => {
    const target = new URL(url);
    const path = target.pathname.endsWith('/zip') ? '/artifact' : target.pathname;
    const response = await fetch(`http://127.0.0.1:${port}${path}`, options);
    if (path === '/artifact' || path === '/storage') {
      assert.equal(response.headers.get('x-request-connection'), 'close');
    }
    return response;
  };
  const transport = readOnlyFetch(localFetch);
  const options = { headers: new Headers({ authorization: 'Bearer test-token', connection: 'keep-alive' }) };
  const first = await (await transport('https://api.github.com/repos/grafana/ai-sdk', options)).json();
  if (artifact) await downloadArtifact(1, 'test-token', localFetch);
  await new Promise(resolve => setImmediate(resolve));
  server.postMessage('close');
  assert.notEqual(Atomics.wait(closed, 0, 0, 5000), 'timed-out');
  const second = await (await transport('https://api.github.com/repos/grafana/ai-sdk', options)).json();
  assert.notEqual(second.connection, first.connection);
  assert.equal(second.auth, 'Bearer test-token');
  assert.equal(options.headers.get('connection'), 'keep-alive');
});
