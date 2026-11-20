import React from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { OrgRole } from '@grafana/data';
import { PluginPageProps } from '@grafana/runtime';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import NewCatalog from './NewCatalog';
import { createModelCatalog, updateModelCatalog } from '../api/modelCatalogs';
import { CatalogActions } from '../permissions';
import { signInAs } from '../testUtils';

jest.mock('../api/modelCatalogs', () => ({
  ...jest.requireActual('../api/modelCatalogs'),
  createModelCatalog: jest.fn(),
  updateModelCatalog: jest.fn(),
}));

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  config: { appUrl: 'https://grafana.example/', namespace: 'stacks-123' },
  PluginPage: ({ children }: PluginPageProps) => <>{children}</>,
}));

const APP = '/a/grafana-aigateway-app';
const URL = 'https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123/modelcatalogs';
const KEY = 'typed-provider-key';
const OTHER_KEY = 'other-provider-key';
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

async function connect(provider: 'Anthropic' | 'OpenAI', key: string) {
  fireEvent.click(screen.getByRole('button', { name: 'Connect a provider' }));
  const dialog = within(screen.getByRole('dialog'));
  fireEvent.click(dialog.getByRole('radio', { name: provider }));
  type(dialog.getByLabelText(/^API key/), key);
  fireEvent.click(dialog.getByRole('button', { name: 'Connect' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
}

async function addModel(provider: string, providerModel: string) {
  const card = within(screen.getByRole('region', { name: provider }));
  type(card.getByRole('textbox', { name: `Add a model to ${provider}` }), providerModel);
  fireEvent.click(card.getByRole('button', { name: 'Add model' }));
  await card.findByRole('listitem', { name: providerModel });
}

describe('pages/NewCatalog', () => {
  beforeEach(() => {
    jest
      .mocked(createModelCatalog)
      .mockReset()
      .mockResolvedValue({} as never);
    jest.mocked(updateModelCatalog).mockReset();
  });

  test('creates a catalog with its providers, keys and models in one request, then opens it', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderPage();

    expect(screen.queryByText(new RegExp(`^${URL}`))).not.toBeInTheDocument();
    type(screen.getByRole('textbox', { name: /^Catalog name/ }), 'ops');
    expect(screen.getByText(`${URL}/ops`)).toBeInTheDocument();
    await connect('Anthropic', KEY);
    expect(within(screen.getByRole('region', { name: 'anthropic' })).getByText('Key entered')).toBeInTheDocument();
    await addModel('anthropic', 'claude-sonnet-4-6');
    expect(createModelCatalog).not.toHaveBeenCalled();
    expect(updateModelCatalog).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Create catalog' }));

    expect(await screen.findByText('catalog page')).toBeInTheDocument();
    expect(createModelCatalog).toHaveBeenCalledWith(
      'ops',
      {
        providers: { anthropic: { type: 'anthropic' } },
        models: {
          'claude-sonnet-4-6': {
            name: 'claude-sonnet-4-6',
            primary: { provider: 'anthropic', model: 'claude-sonnet-4-6' },
          },
        },
      },
      { anthropic: { create: KEY } }
    );
  });

  test('never shows an entered key, and discards the key of a removed provider', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderPage();

    type(screen.getByRole('textbox', { name: /^Catalog name/ }), 'ops');
    await connect('Anthropic', KEY);
    await connect('OpenAI', OTHER_KEY);
    await addModel('openai', 'gpt-5');
    expect(document.body.innerHTML).not.toContain(KEY);
    expect(document.body.innerHTML).not.toContain(OTHER_KEY);

    fireEvent.click(
      within(screen.getByRole('region', { name: 'anthropic' })).getByRole('button', { name: 'Remove anthropic' })
    );
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    await waitFor(() => expect(screen.queryByRole('region', { name: 'anthropic' })).not.toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Create catalog' }));

    await screen.findByText('catalog page');
    const [, spec, secure] = jest.mocked(createModelCatalog).mock.calls[0];
    expect(Object.keys(spec.providers)).toEqual(['openai']);
    expect(secure).toEqual({ openai: { create: OTHER_KEY } });
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
    expect(screen.getByText('Connect a provider and add a model.')).toBeInTheDocument();
    await connect('Anthropic', KEY);
    fireEvent.click(screen.getByRole('button', { name: 'Create catalog' }));
    expect(await screen.findByText('Add at least one model.')).toBeInTheDocument();
    expect(createModelCatalog).not.toHaveBeenCalled();
  });

  test('shows a refused create as returned and keeps the input', async () => {
    const message =
      'admission hook for ModelCatalog denied the request: config: alias "claude-sonnet-4-6" collides with canonical model';
    jest.mocked(createModelCatalog).mockRejectedValue({ status: 400, data: { kind: 'Status', message } });
    signInAs(OrgRole.Admin, ALL);
    renderPage();

    type(screen.getByRole('textbox', { name: /^Catalog name/ }), 'ops');
    await connect('Anthropic', KEY);
    await addModel('anthropic', 'claude-sonnet-4-6');
    fireEvent.click(screen.getByRole('button', { name: 'Create catalog' }));

    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: /^Catalog name/ })).toHaveValue('ops');
    expect(screen.getByRole('listitem', { name: 'claude-sonnet-4-6' })).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent(KEY);
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
