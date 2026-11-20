import React from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { OrgRole } from '@grafana/data';
import { PluginPageProps } from '@grafana/runtime';
import { fireEvent, render, screen } from '@testing-library/react';
import NewCatalog from './NewCatalog';
import { createModelCatalog } from '../api/modelCatalogs';
import { CatalogActions } from '../permissions';
import { signInAs } from '../testUtils';

jest.mock('../api/modelCatalogs', () => ({
  ...jest.requireActual('../api/modelCatalogs'),
  createModelCatalog: jest.fn(),
}));

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  config: { appUrl: 'https://grafana.example/', namespace: 'stacks-123' },
  PluginPage: ({ children }: PluginPageProps) => <>{children}</>,
}));

const APP = '/a/grafana-aigateway-app';
const URL = 'https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123/modelcatalogs';
const ALL = Object.values(CatalogActions);

function renderPage() {
  return render(
    <MemoryRouter initialEntries={[`${APP}/catalogs/new`]}>
      <Routes>
        <Route path={`${APP}/catalogs/new`} element={<NewCatalog />} />
        <Route path={`${APP}/catalogs/edit/:name`} element={<div>catalog page</div>} />
      </Routes>
    </MemoryRouter>
  );
}

function type(element: HTMLElement, value: string) {
  fireEvent.change(element, { target: { value } });
}

describe('pages/NewCatalog', () => {
  beforeEach(() => {
    jest
      .mocked(createModelCatalog)
      .mockReset()
      .mockResolvedValue({} as never);
  });

  test('creates a catalog by name, then opens it', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderPage();

    expect(screen.queryByText(new RegExp(`^${URL}`))).not.toBeInTheDocument();
    type(screen.getByRole('textbox', { name: /^Catalog name/ }), 'ops');
    expect(screen.getByText(`${URL}/ops`)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Connect a provider' })).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
    expect(createModelCatalog).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Create catalog' }));

    expect(await screen.findByText('catalog page')).toBeInTheDocument();
    expect(createModelCatalog).toHaveBeenCalledTimes(1);
    expect(createModelCatalog).toHaveBeenCalledWith('ops');
  });

  test('checks the name as it is typed and shows the URL only for a valid one', () => {
    signInAs(OrgRole.Admin, ALL);
    renderPage();

    type(screen.getByRole('textbox', { name: /^Catalog name/ }), '$%^&*');
    expect(screen.getByText("Use only letters, digits and '-', '_', '.' or ':'.")).toBeInTheDocument();
    expect(screen.queryByText(new RegExp(`^${URL}`))).not.toBeInTheDocument();
    type(screen.getByRole('textbox', { name: /^Catalog name/ }), 'team');
    expect(screen.getByText(`${URL}/team`)).toBeInTheDocument();
  });

  test('says what is missing without a request', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderPage();

    fireEvent.click(screen.getByRole('button', { name: 'Create catalog' }));

    expect(await screen.findByText('Enter a name.')).toBeInTheDocument();
    expect(createModelCatalog).not.toHaveBeenCalled();
  });

  test('shows a refused create as returned and keeps the input', async () => {
    const message = 'modelcatalogs.aigateway.ext.grafana.app "ops" already exists';
    jest.mocked(createModelCatalog).mockRejectedValue({ status: 400, data: { kind: 'Status', message } });
    signInAs(OrgRole.Admin, ALL);
    renderPage();

    type(screen.getByRole('textbox', { name: /^Catalog name/ }), 'ops');
    fireEvent.click(screen.getByRole('button', { name: 'Create catalog' }));

    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: /^Catalog name/ })).toHaveValue('ops');
    expect(screen.queryByText('catalog page')).not.toBeInTheDocument();
  });

  test('offers no catalog creation without the create action', () => {
    signInAs(OrgRole.Editor, [CatalogActions.Read]);
    renderPage();

    expect(screen.getByText("You can't create model catalogs")).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Create catalog' })).not.toBeInTheDocument();
  });

  test('shows nothing without the read action', () => {
    signInAs(OrgRole.Admin, []);
    renderPage();

    expect(screen.getByText("You can't view model catalogs")).toBeInTheDocument();
  });
});
