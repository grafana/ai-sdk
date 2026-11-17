import React from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { OrgRole } from '@grafana/data';
import { PluginPageProps } from '@grafana/runtime';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import CatalogList from './CatalogList';
import { createModelCatalog, deleteModelCatalog, listModelCatalogs } from '../api/modelCatalogs';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { CatalogActions } from '../permissions';
import { signInAs } from '../testUtils';

jest.mock('../api/modelCatalogs', () => ({
  ...jest.requireActual('../api/modelCatalogs'),
  listModelCatalogs: jest.fn(),
  createModelCatalog: jest.fn(),
  deleteModelCatalog: jest.fn(),
}));

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  config: { appUrl: 'https://grafana.example/', namespace: 'stacks-123' },
  PluginPage: ({ subTitle, actions, children }: PluginPageProps) => (
    <>
      {subTitle}
      {actions}
      {children}
    </>
  ),
}));

const APP = '/a/grafana-aigateway-app';
const URL = 'https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123/modelcatalogs';
const TYPED_KEY = 'typed-provider-key';
const READ_ONLY = [CatalogActions.Read];
const ALL = Object.values(CatalogActions);
const EMPTY_STATE_TIMEOUT_MS = 15_000;

function catalog(name: string): ModelCatalog {
  return {
    apiVersion: 'aigateway.ext.grafana.app/v1alpha1',
    kind: 'ModelCatalog',
    metadata: { name, namespace: 'stacks-123' },
    spec: {
      providers: { claude: { type: 'anthropic' }, gpt: { type: 'openai' } },
      models: { balanced: { name: 'Balanced', primary: { provider: 'claude', model: 'claude-sonnet-4-6' } } },
    },
    secure: { claude: { name: 'inline-secret-name' }, gpt: { name: 'inline-secret-gpt' } },
  };
}

function renderList() {
  return render(
    <MemoryRouter initialEntries={[`${APP}/catalogs`]}>
      <Routes>
        <Route path={`${APP}/catalogs`} element={<CatalogList />} />
        <Route path={`${APP}/catalogs/:name`} element={<div>catalog page</div>} />
      </Routes>
    </MemoryRouter>
  );
}

function type(element: HTMLElement, value: string) {
  fireEvent.change(element, { target: { value } });
}

function openNewCatalog() {
  fireEvent.click(screen.getByRole('button', { name: 'New catalog' }));
  return within(screen.getByRole('dialog'));
}

describe('pages/CatalogList', () => {
  beforeEach(() => {
    jest
      .mocked(listModelCatalogs)
      .mockReset()
      .mockResolvedValue([catalog('team')]);
    jest.mocked(createModelCatalog).mockReset().mockResolvedValue(catalog('team'));
    jest.mocked(deleteModelCatalog).mockReset().mockResolvedValue();
  });

  test('shows each catalog with its URL, counts and provider types, never secure names', async () => {
    signInAs(OrgRole.Viewer, READ_ONLY);
    renderList();

    expect(await screen.findByRole('link', { name: 'team' })).toHaveAttribute('href', `${APP}/catalogs/team`);
    expect(screen.getByText(`${URL}/team`)).toBeInTheDocument();
    expect(screen.getByText('2 providers')).toBeInTheDocument();
    expect(screen.getByText('1 model')).toBeInTheDocument();
    expect(screen.getByText('Anthropic')).toBeInTheDocument();
    expect(screen.getByText('OpenAI')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy URL' })).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent('inline-secret');
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
    expect(screen.queryByRole('button', { name: 'New catalog' }) !== null).toBe(writes);
    expect(screen.queryByRole('button', { name: 'Delete team' }) !== null).toBe(writes);
  });

  test('requests nothing without the read action', () => {
    signInAs(OrgRole.Admin, []);
    renderList();

    expect(screen.getByText("You can't view model catalogs")).toBeInTheDocument();
    expect(listModelCatalogs).not.toHaveBeenCalled();
  });

  test.each([
    ['a creator', ALL, true],
    ['a reader', READ_ONLY, false],
  ])(
    'offers to create the first catalog to %s: %s',
    async (_, actions, offered) => {
      jest.mocked(listModelCatalogs).mockResolvedValue([]);
      signInAs(OrgRole.Admin, actions);
      renderList();

      expect(await screen.findByText('No model catalogs yet')).toBeInTheDocument();
      expect(screen.queryAllByRole('button', { name: 'New catalog' })).toHaveLength(offered ? 1 : 0);
    },
    EMPTY_STATE_TIMEOUT_MS
  );

  test('creates a catalog with its first provider and model, then opens it', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderList();
    await screen.findByRole('link', { name: 'team' });

    const dialog = openNewCatalog();
    type(dialog.getByRole('textbox', { name: /^Catalog name/ }), 'ops');
    expect(dialog.getByText(`${URL}/ops`)).toBeInTheDocument();
    type(dialog.getByLabelText(/^API key/), TYPED_KEY);
    type(dialog.getByRole('textbox', { name: /^Model ID/ }), 'balanced');
    type(dialog.getByRole('textbox', { name: /^Display name/ }), 'Balanced');
    type(dialog.getByRole('textbox', { name: /^Provider model/ }), 'claude-sonnet-4-6');
    fireEvent.click(dialog.getByRole('button', { name: 'Create catalog' }));

    expect(await screen.findByText('catalog page')).toBeInTheDocument();
    expect(createModelCatalog).toHaveBeenCalledWith(
      'ops',
      {
        providers: { anthropic: { type: 'anthropic' } },
        models: { balanced: { name: 'Balanced', primary: { provider: 'anthropic', model: 'claude-sonnet-4-6' } } },
      },
      { anthropic: { create: TYPED_KEY } }
    );
    expect(document.body).not.toHaveTextContent(TYPED_KEY);
  });

  test('names a new provider after its type until the writer renames it', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderList();
    await screen.findByRole('link', { name: 'team' });

    const dialog = openNewCatalog();
    const name = dialog.getByRole('textbox', { name: /^Name/ });
    fireEvent.click(dialog.getByRole('radio', { name: 'OpenAI' }));
    expect(name).toHaveValue('openai');
    type(name, 'team-openai');
    fireEvent.click(dialog.getByRole('radio', { name: 'Anthropic' }));
    expect(name).toHaveValue('team-openai');
  });

  test('marks missing fields without a request', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderList();
    await screen.findByRole('link', { name: 'team' });

    const dialog = openNewCatalog();
    fireEvent.click(dialog.getByRole('button', { name: 'Create catalog' }));

    for (const message of [
      'Enter a name.',
      'Enter the API key.',
      'Enter a model ID.',
      'Enter a display name.',
      "Enter the provider's model ID.",
    ]) {
      expect(await dialog.findByText(message)).toBeInTheDocument();
    }
    expect(createModelCatalog).not.toHaveBeenCalled();
  });

  test('shows a refused create as returned and keeps the input', async () => {
    const message = 'admission hook for ModelCatalog denied the request: config: model "bad id": public ID';
    jest.mocked(createModelCatalog).mockRejectedValue({ status: 400, data: { kind: 'Status', message } });
    signInAs(OrgRole.Admin, ALL);
    renderList();
    await screen.findByRole('link', { name: 'team' });

    const dialog = openNewCatalog();
    type(dialog.getByRole('textbox', { name: /^Catalog name/ }), 'ops');
    type(dialog.getByLabelText(/^API key/), TYPED_KEY);
    type(dialog.getByRole('textbox', { name: /^Model ID/ }), 'bad id');
    type(dialog.getByRole('textbox', { name: /^Display name/ }), 'Bad');
    type(dialog.getByRole('textbox', { name: /^Provider model/ }), 'claude-sonnet-4-6');
    fireEvent.click(dialog.getByRole('button', { name: 'Create catalog' }));

    expect(await dialog.findByText(message)).toBeInTheDocument();
    expect(dialog.getByRole('textbox', { name: /^Model ID/ })).toHaveValue('bad id');
    expect(document.body).not.toHaveTextContent(TYPED_KEY);
    expect(screen.queryByText('catalog page')).not.toBeInTheDocument();
  });

  test(
    'deletes a catalog only after confirmation',
    async () => {
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
    },
    EMPTY_STATE_TIMEOUT_MS
  );

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
