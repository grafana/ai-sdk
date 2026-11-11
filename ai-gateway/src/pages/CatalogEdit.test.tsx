import React from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { OrgRole } from '@grafana/data';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import CatalogEdit from './CatalogEdit';
import { createModelCatalog, getModelCatalog, updateModelCatalog } from '../api/modelCatalogs';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { CatalogActions } from '../permissions';
import { chooseOption, signInAs } from '../testUtils';

jest.mock('../api/modelCatalogs', () => ({
  ...jest.requireActual('../api/modelCatalogs'),
  getModelCatalog: jest.fn(),
  createModelCatalog: jest.fn(),
  updateModelCatalog: jest.fn(),
}));

const APP = '/a/grafana-aigateway-app';
const TYPED_KEY = 'typed-provider-key';
const READ_ONLY = [CatalogActions.Read];
const ALL = Object.values(CatalogActions);

const stored: ModelCatalog = {
  apiVersion: 'aigateway.ext.grafana.app/v1alpha1',
  kind: 'ModelCatalog',
  metadata: { name: 'team', namespace: 'default', resourceVersion: '7' },
  spec: {
    providers: { claude: { type: 'anthropic' }, gpt: { type: 'openai' } },
    models: {
      balanced: {
        name: 'Balanced',
        aliases: ['default'],
        primary: { provider: 'claude', model: 'claude-sonnet-4-6' },
        fallback: [{ provider: 'gpt', model: 'gpt-5' }],
      },
    },
  },
  secure: { claude: { name: 'inline-claude' }, gpt: { name: 'inline-gpt' } },
};

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path={`${APP}/catalogs`} element={<div>catalog list</div>} />
        <Route path={`${APP}/catalogs/new`} element={<CatalogEdit />} />
        <Route path={`${APP}/catalogs/edit/:name`} element={<CatalogEdit />} />
      </Routes>
    </MemoryRouter>
  );
}

function input(name: string) {
  return screen.getByRole('textbox', { name });
}

function type(element: HTMLElement, value: string) {
  fireEvent.change(element, { target: { value } });
}

async function openStored() {
  renderAt(`${APP}/catalogs/edit/team`);
  await screen.findByDisplayValue('Balanced');
}

describe('pages/CatalogEdit', () => {
  beforeEach(() => {
    jest.mocked(getModelCatalog).mockReset().mockResolvedValue(stored);
    jest.mocked(createModelCatalog).mockReset().mockResolvedValue(stored);
    jest.mocked(updateModelCatalog).mockReset().mockResolvedValue(stored);
  });

  test('creates a catalog with its key as secure.create and never shows the key again', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderAt(`${APP}/catalogs/new`);

    type(input('Catalog name'), 'team');
    type(input('Provider 1 name'), 'claude');
    type(screen.getByLabelText('Provider 1 API key'), TYPED_KEY);
    type(input('Model 1 ID'), 'balanced');
    type(input('Model 1 display name'), 'Balanced');
    chooseOption(screen.getByRole('combobox', { name: 'Model 1 primary provider' }), 'claude');
    type(input('Model 1 primary model'), 'claude-sonnet-4-6');
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(await screen.findByText('catalog list')).toBeInTheDocument();
    expect(createModelCatalog).toHaveBeenCalledWith(
      'team',
      {
        providers: { claude: { type: 'anthropic' } },
        models: { balanced: { name: 'Balanced', primary: { provider: 'claude', model: 'claude-sonnet-4-6' } } },
      },
      { claude: { create: TYPED_KEY } }
    );
    expect(document.body.innerHTML).not.toContain(TYPED_KEY);
  });

  test('shows stored keys as configured and omits them from an edit', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openStored();

    expect(screen.getAllByDisplayValue('configured')).toHaveLength(2);
    expect(document.body.innerHTML).not.toContain('inline-claude');
    type(input('Model 1 display name'), 'Renamed');
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await screen.findByText('catalog list');
    const [catalog, spec, secure] = jest.mocked(updateModelCatalog).mock.calls[0];
    expect(catalog).toBe(stored);
    expect(spec.models.balanced).toEqual({ ...stored.spec.models.balanced, name: 'Renamed' });
    expect(secure).toEqual({});
  });

  test('sends only a replaced key', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openStored();

    fireEvent.click(screen.getAllByRole('button', { name: 'Reset' })[0]);
    type(screen.getByLabelText('Provider 1 API key'), TYPED_KEY);
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await screen.findByText('catalog list');
    expect(jest.mocked(updateModelCatalog).mock.calls[0][2]).toEqual({ claude: { create: TYPED_KEY } });
  });

  test('removes the key of a removed provider', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openStored();

    fireEvent.click(screen.getByRole('button', { name: 'Remove provider 2' }));
    fireEvent.click(screen.getByRole('button', { name: 'Remove fallback 1' }));
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await screen.findByText('catalog list');
    const [, spec, secure] = jest.mocked(updateModelCatalog).mock.calls[0];
    expect(Object.keys(spec.providers)).toEqual(['claude']);
    expect(spec.models.balanced.fallback).toBeUndefined();
    expect(secure).toEqual({ gpt: { remove: true } });
  });

  test.each([
    ['a Viewer', OrgRole.Viewer, READ_ONLY, true],
    ['an Editor without the writer role', OrgRole.Editor, READ_ONLY, true],
    ['an Admin without the update action', OrgRole.Admin, [CatalogActions.Read, CatalogActions.Create], true],
    ['an Editor holding the update action', OrgRole.Editor, [CatalogActions.Read, CatalogActions.Update], false],
  ])('shows a catalog to %s read-only: %s', async (_, role, actions, readOnly) => {
    signInAs(role, actions);
    await openStored();

    expect(screen.queryByRole('button', { name: 'Save' }) === null).toBe(readOnly);
    expect(screen.queryByRole('button', { name: 'Add provider' }) === null).toBe(readOnly);
    expect(screen.queryAllByRole('button', { name: 'Reset' }).length === 0).toBe(readOnly);
    expect(input('Model 1 display name').hasAttribute('disabled')).toBe(readOnly);
    expect(input('Catalog name')).toBeDisabled();
  });

  test('offers no catalog creation without the create action', () => {
    signInAs(OrgRole.Editor, READ_ONLY);
    renderAt(`${APP}/catalogs/new`);

    expect(screen.getByText("You can't create model catalogs")).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument();
  });

  test('requests nothing without the read action', () => {
    signInAs(OrgRole.Admin, []);
    renderAt(`${APP}/catalogs/edit/team`);

    expect(screen.getByText("You can't view model catalogs")).toBeInTheDocument();
    expect(getModelCatalog).not.toHaveBeenCalled();
  });

  test('shows an admission refusal as returned and keeps the form', async () => {
    const message = 'admission hook for ModelCatalog denied the request: models.balanced.primary.provider: unknown';
    jest.mocked(updateModelCatalog).mockRejectedValue({ status: 400, data: { kind: 'Status', message } });
    signInAs(OrgRole.Admin, ALL);
    await openStored();

    fireEvent.click(screen.getAllByRole('button', { name: 'Reset' })[0]);
    type(screen.getByLabelText('Provider 1 API key'), TYPED_KEY);
    type(input('Model 1 display name'), 'Renamed');
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(input('Model 1 display name')).toHaveValue('Renamed');
    expect(document.body).not.toHaveTextContent(TYPED_KEY);
    expect(screen.queryByText('catalog list')).not.toBeInTheDocument();
  });

  test('shows a failed load as returned', async () => {
    jest.mocked(getModelCatalog).mockRejectedValue({ status: 404, data: { message: 'modelcatalogs "team" not found' } });
    signInAs(OrgRole.Viewer, READ_ONLY);
    renderAt(`${APP}/catalogs/edit/team`);

    expect(await screen.findByText('modelcatalogs "team" not found')).toBeInTheDocument();
  });

  test('refuses duplicate provider names without a request', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openStored();

    type(input('Provider 2 name'), 'claude');
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(
      await screen.findByText('Provider names must be unique: "claude" is used more than once.')
    ).toBeInTheDocument();
    await waitFor(() => expect(updateModelCatalog).not.toHaveBeenCalled());
  });
});
