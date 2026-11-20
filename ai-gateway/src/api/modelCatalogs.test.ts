import { config, getBackendSrv } from '@grafana/runtime';
import {
  createModelCatalog,
  deleteModelCatalog,
  errorMessage,
  getModelCatalog,
  listModelCatalogs,
  modelCatalogsURL,
  updateModelCatalog,
} from './modelCatalogs';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';

const CATALOGS = '/grafana/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123/modelcatalogs';
const OPTIONS = { showErrorAlert: false };

const backendSrv = {
  get: jest.fn(),
  post: jest.fn(),
  put: jest.fn(),
  delete: jest.fn(),
};

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  config: { appSubUrl: '/grafana', namespace: 'stacks-123' },
  getBackendSrv: jest.fn(),
}));

const spec = {
  providers: { anthropic: { type: 'anthropic' } },
  models: { balanced: { name: 'Balanced', primary: { provider: 'anthropic', model: 'claude' } } },
};

const stored: ModelCatalog = {
  apiVersion: 'aigateway.ext.grafana.app/v1alpha1',
  kind: 'ModelCatalog',
  metadata: { name: 'team', namespace: 'stacks-123', resourceVersion: '42', labels: { owner: 'ops' } },
  spec,
  secure: { anthropic: { name: 'inline-abc' } },
};

describe('api/modelCatalogs', () => {
  beforeEach(() => {
    jest.mocked(getBackendSrv).mockReturnValue(backendSrv as unknown as ReturnType<typeof getBackendSrv>);
    Object.values(backendSrv).forEach((fn) => fn.mockReset());
  });

  test.each([
    ['/grafana', 'stacks-123', CATALOGS],
    ['', 'org 2', '/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/org%202/modelcatalogs'],
  ])('builds the collection path from appSubUrl %j and namespace %j', (appSubUrl, namespace, expected) => {
    const original = { ...config };
    Object.assign(config, { appSubUrl, namespace });
    expect(modelCatalogsURL()).toBe(expected);
    Object.assign(config, original);
  });

  test('lists catalogs without a global error alert', async () => {
    backendSrv.get.mockResolvedValue({ items: [stored] });
    await expect(listModelCatalogs()).resolves.toEqual([stored]);
    expect(backendSrv.get).toHaveBeenCalledWith(CATALOGS, undefined, undefined, OPTIONS);
  });

  test('lists an empty collection without items', async () => {
    backendSrv.get.mockResolvedValue({});
    await expect(listModelCatalogs()).resolves.toEqual([]);
  });

  test('gets one catalog by name', async () => {
    backendSrv.get.mockResolvedValue(stored);
    await expect(getModelCatalog('team')).resolves.toBe(stored);
    expect(backendSrv.get).toHaveBeenCalledWith(`${CATALOGS}/team`, undefined, undefined, OPTIONS);
  });

  test('creates a catalog with its keys as secure.create', async () => {
    await createModelCatalog('team', spec, { anthropic: { create: 'key' } });
    expect(backendSrv.post).toHaveBeenCalledWith(
      CATALOGS,
      {
        apiVersion: 'aigateway.ext.grafana.app/v1alpha1',
        kind: 'ModelCatalog',
        metadata: { name: 'team' },
        spec,
        secure: { anthropic: { create: 'key' } },
      },
      OPTIONS
    );
  });

  test('creates a catalog without secure when no key is entered', async () => {
    await createModelCatalog('team', spec, {});
    expect(backendSrv.post.mock.calls[0][1]).not.toHaveProperty('secure');
  });

  test('updates a catalog with its stored metadata and only the changed secure values', async () => {
    const changed = { ...spec, models: {} };
    await updateModelCatalog(stored, changed, {});
    expect(backendSrv.put).toHaveBeenCalledWith(
      `${CATALOGS}/team`,
      { apiVersion: stored.apiVersion, kind: stored.kind, metadata: stored.metadata, spec: changed },
      OPTIONS
    );

    await updateModelCatalog(stored, spec, { anthropic: { create: 'new-key' }, openai: { remove: true } });
    expect(backendSrv.put.mock.calls[1][1].secure).toEqual({
      anthropic: { create: 'new-key' },
      openai: { remove: true },
    });
  });

  test('deletes a catalog by name', async () => {
    await deleteModelCatalog('team');
    expect(backendSrv.delete).toHaveBeenCalledWith(`${CATALOGS}/team`, undefined, OPTIONS);
  });

  test.each([
    [
      'a Status message',
      { status: 400, data: { kind: 'Status', message: 'admission webhook denied: providers.x' } },
      'admission webhook denied: providers.x',
    ],
    ['the status text', { status: 502, statusText: 'Bad Gateway', data: '' }, 'Bad Gateway'],
    ['the status code', { status: 500, data: {} }, 'Request failed with status 500'],
    ['an error message', new Error('network down'), 'network down'],
  ])('returns %s as the error message', (_, error, expected) => {
    expect(errorMessage(error)).toBe(expected);
  });
});
