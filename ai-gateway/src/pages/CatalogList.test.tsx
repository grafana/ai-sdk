import React from 'react';
import { MemoryRouter } from 'react-router-dom';
import { OrgRole } from '@grafana/data';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import CatalogList from './CatalogList';
import { deleteModelCatalog, listModelCatalogs } from '../api/modelCatalogs';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { CatalogActions } from '../permissions';
import { signInAs } from '../testUtils';

jest.mock('../api/modelCatalogs', () => ({
  ...jest.requireActual('../api/modelCatalogs'),
  listModelCatalogs: jest.fn(),
  deleteModelCatalog: jest.fn(),
}));

const READ_ONLY = [CatalogActions.Read];
const ALL = Object.values(CatalogActions);

function catalog(name: string): ModelCatalog {
  return {
    apiVersion: 'aigateway.ext.grafana.app/v1alpha1',
    kind: 'ModelCatalog',
    metadata: { name, namespace: 'default' },
    spec: {
      providers: { claude: { type: 'anthropic' } },
      models: { balanced: { name: 'Balanced', primary: { provider: 'claude', model: 'claude-sonnet-4-6' } } },
    },
    secure: { claude: { name: 'inline-secret-name' } },
  };
}

function renderList() {
  return render(
    <MemoryRouter>
      <CatalogList />
    </MemoryRouter>
  );
}

describe('pages/CatalogList', () => {
  beforeEach(() => {
    jest
      .mocked(listModelCatalogs)
      .mockReset()
      .mockResolvedValue([catalog('team')]);
    jest.mocked(deleteModelCatalog).mockReset().mockResolvedValue();
  });

  test('lists catalogs with their providers and models, never secure names', async () => {
    signInAs(OrgRole.Viewer, READ_ONLY);
    renderList();

    expect(await screen.findByRole('link', { name: 'team' })).toHaveAttribute(
      'href',
      '/a/grafana-aigateway-app/catalogs/edit/team'
    );
    expect(screen.getByText('claude (anthropic)')).toBeInTheDocument();
    expect(screen.getByText('balanced')).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent('inline-secret-name');
  });

  test.each([
    ['a Viewer', OrgRole.Viewer, READ_ONLY, false],
    ['an Editor without the writer role', OrgRole.Editor, READ_ONLY, false],
    ['an Admin without write actions', OrgRole.Admin, READ_ONLY, false],
    ['an Editor holding the writer actions', OrgRole.Editor, ALL, true],
  ])('offers create and delete to %s only with the actions', async (_, role, actions, writes) => {
    signInAs(role, actions);
    renderList();

    await screen.findByRole('link', { name: 'team' });
    expect(screen.queryByRole('link', { name: /New catalog/ }) !== null).toBe(writes);
    expect(screen.queryByRole('button', { name: 'Delete team' }) !== null).toBe(writes);
  });

  test('requests nothing without the read action', async () => {
    signInAs(OrgRole.Admin, []);
    renderList();

    expect(screen.getByText("You can't view model catalogs")).toBeInTheDocument();
    expect(listModelCatalogs).not.toHaveBeenCalled();
  });

  test('offers to create the first catalog only to creators', async () => {
    jest.mocked(listModelCatalogs).mockResolvedValue([]);
    signInAs(OrgRole.Admin, ALL);
    const { unmount } = renderList();
    expect(await screen.findByText('No model catalogs yet')).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: /New catalog/ }).length).toBeGreaterThan(0);
    unmount();

    signInAs(OrgRole.Viewer, READ_ONLY);
    renderList();
    expect(await screen.findByText('No model catalogs yet')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: /New catalog/ })).not.toBeInTheDocument();
  });

  test('deletes a catalog only after confirmation', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderList();

    fireEvent.click(await screen.findByRole('button', { name: 'Delete team' }));
    expect(deleteModelCatalog).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(deleteModelCatalog).not.toHaveBeenCalled();

    jest.mocked(listModelCatalogs).mockResolvedValue([]);
    fireEvent.click(screen.getByRole('button', { name: 'Delete team' }));
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

    await waitFor(() => expect(deleteModelCatalog).toHaveBeenCalledWith('team'));
    expect(await screen.findByText('No model catalogs yet')).toBeInTheDocument();
  });

  test('shows the message of a refused delete as returned', async () => {
    jest.mocked(deleteModelCatalog).mockRejectedValue({ status: 403, data: { message: 'forbidden: catalogwriter' } });
    signInAs(OrgRole.Admin, ALL);
    renderList();

    fireEvent.click(await screen.findByRole('button', { name: 'Delete team' }));
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

    expect(await screen.findByText('forbidden: catalogwriter')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'team' })).toBeInTheDocument();
  });

  test('shows the message of a failed list as returned', async () => {
    jest.mocked(listModelCatalogs).mockRejectedValue({ status: 500, data: { message: 'storage unavailable' } });
    signInAs(OrgRole.Viewer, READ_ONLY);
    renderList();

    expect(await screen.findByText('storage unavailable')).toBeInTheDocument();
  });
});
