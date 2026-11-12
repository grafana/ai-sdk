import React from 'react';
import { MemoryRouter } from 'react-router-dom';
import { AppRootProps, OrgRole, PluginType } from '@grafana/data';
import { render, screen, waitFor } from '@testing-library/react';
import App from './App';
import { listModelCatalogs, getModelCatalog } from '../../api/modelCatalogs';
import { CatalogActions } from '../../permissions';
import { signInAs } from '../../testUtils';

jest.mock('../../api/modelCatalogs', () => ({
  ...jest.requireActual('../../api/modelCatalogs'),
  listModelCatalogs: jest.fn(),
  getModelCatalog: jest.fn(),
}));

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  config: { appUrl: 'https://grafana.example/', namespace: 'stacks-123' },
}));

describe('Components/App', () => {
  let props: AppRootProps;

  beforeEach(() => {
    jest.clearAllMocks();
    jest.mocked(listModelCatalogs).mockResolvedValue([]);
    jest.mocked(getModelCatalog).mockResolvedValue({
      apiVersion: 'aigateway.ext.grafana.app/v1alpha1',
      kind: 'ModelCatalog',
      metadata: { name: 'team', namespace: 'stacks-123' },
      spec: { providers: {}, models: {} },
    });
    signInAs(OrgRole.Admin, Object.values(CatalogActions));

    props = {
      basename: 'a/grafana-aigateway-app',
      meta: {
        id: 'grafana-aigateway-app',
        name: 'Grafana AI Gateway',
        type: PluginType.app,
        enabled: true,
        jsonData: {},
      },
      query: {},
      path: '',
      onNavChanged: jest.fn(),
    } as unknown as AppRootProps;
  });

  test.each(['/', '/overview', '/unknown'])('renders the overview at %s', async (path) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <App {...props} />
      </MemoryRouter>
    );

    await waitFor(() => expect(screen.getByText('Quick start')).toBeInTheDocument(), { timeout: 2000 });
  });

  test.each([
    ['/catalogs', () => screen.findByText('No model catalogs yet')],
    ['/catalogs/new', () => screen.findByRole('button', { name: 'Add provider' })],
    ['/catalogs/edit/team', () => screen.findByDisplayValue('team')],
  ])('renders the catalog page at %s', async (path, find) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <App {...props} />
      </MemoryRouter>
    );

    expect(await find()).toBeInTheDocument();
    expect(screen.queryByText('Quick start')).not.toBeInTheDocument();
  });
});
