import React from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { OrgRole } from '@grafana/data';
import { PluginPageProps } from '@grafana/runtime';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import CatalogPage from './CatalogPage';
import { deleteModelCatalog, getModelCatalog, SecureChanges, updateModelCatalog } from '../api/modelCatalogs';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { Spec } from '../generated/modelcatalog/v1alpha1/types.spec.gen';
import { CatalogActions } from '../permissions';
import { chooseOption, signInAs } from '../testUtils';

jest.mock('../api/modelCatalogs', () => ({
  ...jest.requireActual('../api/modelCatalogs'),
  getModelCatalog: jest.fn(),
  updateModelCatalog: jest.fn(),
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
const TYPED_KEY = 'typed-provider-key';
const READ_ONLY = [CatalogActions.Read];
const ALL = Object.values(CatalogActions);

const stored: ModelCatalog = {
  apiVersion: 'aigateway.ext.grafana.app/v1alpha1',
  kind: 'ModelCatalog',
  metadata: { name: 'team', namespace: 'stacks-123', resourceVersion: '7' },
  spec: {
    providers: { claude: { type: 'anthropic' }, gpt: { type: 'openai' }, spare: { type: 'openai' } },
    models: {
      balanced: {
        name: 'Balanced',
        aliases: ['default'],
        primary: { provider: 'claude', model: 'claude-sonnet-4-6' },
        fallback: [{ provider: 'gpt', model: 'gpt-5' }],
      },
      plain: { name: 'Plain', primary: { provider: 'gpt', model: 'gpt-5-mini' } },
    },
  },
  secure: { claude: { name: 'inline-claude' }, gpt: { name: 'inline-gpt' }, spare: { name: 'inline-spare' } },
};

function updated(catalog: ModelCatalog, spec: Spec, secure: SecureChanges): ModelCatalog {
  const names = Object.fromEntries(
    Object.entries(catalog.secure ?? {}).filter(([provider]) => !secure[provider]?.remove)
  );
  for (const [provider, change] of Object.entries(secure)) {
    if (change.create) {
      names[provider] = { name: `inline-${provider}-new` };
    }
  }
  return { ...catalog, metadata: { ...catalog.metadata, resourceVersion: '8' }, spec, secure: names };
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={[`${APP}/catalogs/team`]}>
      <Routes>
        <Route path={`${APP}/catalogs`} element={<div>catalog list</div>} />
        <Route path={`${APP}/catalogs/:name`} element={<CatalogPage />} />
      </Routes>
    </MemoryRouter>
  );
}

async function openPage() {
  renderPage();
  await screen.findByRole('listitem', { name: 'balanced' });
}

function row(name: string) {
  return within(screen.getByRole('listitem', { name }));
}

function dialog() {
  return within(screen.getByRole('dialog'));
}

function type(element: HTMLElement, value: string) {
  fireEvent.change(element, { target: { value } });
}

function lastUpdate() {
  const calls = jest.mocked(updateModelCatalog).mock.calls;
  return calls[calls.length - 1];
}

describe('pages/CatalogPage', () => {
  beforeEach(() => {
    jest.mocked(getModelCatalog).mockReset().mockResolvedValue(stored);
    jest
      .mocked(updateModelCatalog)
      .mockReset()
      .mockImplementation((catalog, spec, secure) => Promise.resolve(updated(catalog, spec, secure)));
    jest.mocked(deleteModelCatalog).mockReset().mockResolvedValue();
  });

  test('shows the catalog URL, providers with saved keys, and each model route, never secure names', async () => {
    signInAs(OrgRole.Viewer, READ_ONLY);
    await openPage();

    expect(
      screen.getByText(
        'https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123/modelcatalogs/team'
      )
    ).toBeInTheDocument();
    expect(row('claude').getByText('Anthropic')).toBeInTheDocument();
    expect(row('claude').getByText('Key saved')).toBeInTheDocument();
    expect(row('claude').getByText('Used by balanced')).toBeInTheDocument();
    expect(row('spare').getByText('No model uses this provider')).toBeInTheDocument();
    expect(row('balanced').getByText('Calls claude › claude-sonnet-4-6, then gpt › gpt-5')).toBeInTheDocument();
    expect(row('balanced').getByText('Aliases: default')).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent('inline-');
  });

  test.each([
    ['a Viewer', OrgRole.Viewer, READ_ONLY, false, false],
    ['an Editor without the writer role', OrgRole.Editor, READ_ONLY, false, false],
    ['an Admin without the update action', OrgRole.Admin, [CatalogActions.Read, CatalogActions.Delete], false, true],
    ['an Editor holding the update action', OrgRole.Editor, [CatalogActions.Read, CatalogActions.Update], true, false],
  ])('offers changes to %s only with the actions', async (_, role, actions, update, remove) => {
    signInAs(role, actions);
    await openPage();

    expect(screen.queryByRole('button', { name: 'Add provider' }) !== null).toBe(update);
    expect(screen.queryByRole('button', { name: 'Add model' }) !== null).toBe(update);
    expect(screen.queryByRole('button', { name: 'Edit claude' }) !== null).toBe(update);
    expect(screen.queryByRole('button', { name: 'Edit balanced' }) !== null).toBe(update);
    expect(screen.queryByText('Read-only') === null).toBe(update);
    expect(screen.queryByRole('button', { name: 'Delete catalog' }) !== null).toBe(remove);
  });

  test('adds a provider with its key as secure.create and never shows the key', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(screen.getByRole('button', { name: 'Add provider' }));
    type(dialog().getByRole('textbox', { name: /^Name/ }), 'extra');
    type(dialog().getByLabelText(/^API key/), TYPED_KEY);
    fireEvent.click(dialog().getByRole('button', { name: 'Add provider' }));

    expect(await screen.findByRole('listitem', { name: 'extra' })).toBeInTheDocument();
    const [catalog, spec, secure] = lastUpdate();
    expect(catalog).toBe(stored);
    expect(spec).toEqual({ ...stored.spec, providers: { ...stored.spec.providers, extra: { type: 'anthropic' } } });
    expect(secure).toEqual({ extra: { create: TYPED_KEY } });
    expect(row('extra').getByText('Key saved')).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(document.body.innerHTML).not.toContain(TYPED_KEY);
  });

  test('refuses a provider name the catalog has without a request', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(screen.getByRole('button', { name: 'Add provider' }));
    type(dialog().getByRole('textbox', { name: /^Name/ }), 'claude');
    type(dialog().getByLabelText(/^API key/), TYPED_KEY);
    fireEvent.click(dialog().getByRole('button', { name: 'Add provider' }));

    expect(await dialog().findByText('A provider named "claude" already exists.')).toBeInTheDocument();
    expect(updateModelCatalog).not.toHaveBeenCalled();
  });

  test('keeps a saved key when a provider is saved without a new one', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(screen.getByRole('button', { name: 'Edit claude' }));
    expect(dialog().getByRole('textbox', { name: /^Name/ })).toBeDisabled();
    expect(dialog().getByDisplayValue('configured')).toBeInTheDocument();
    fireEvent.click(dialog().getByRole('button', { name: 'Save provider' }));

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    const [, spec, secure] = lastUpdate();
    expect(spec).toEqual(stored.spec);
    expect(secure).toEqual({});
  });

  test('sends only a replaced key', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(screen.getByRole('button', { name: 'Edit claude' }));
    fireEvent.click(dialog().getByRole('button', { name: 'Reset' }));
    fireEvent.click(dialog().getByRole('button', { name: 'Save provider' }));
    expect(await dialog().findByText('Enter the API key.')).toBeInTheDocument();
    expect(updateModelCatalog).not.toHaveBeenCalled();

    type(dialog().getByLabelText(/^API key/), TYPED_KEY);
    fireEvent.click(dialog().getByRole('button', { name: 'Save provider' }));

    await waitFor(() => expect(updateModelCatalog).toHaveBeenCalled());
    expect(lastUpdate()[2]).toEqual({ claude: { create: TYPED_KEY } });
  });

  test('asks for a new key when a stored provider changes type', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(screen.getByRole('button', { name: 'Edit claude' }));
    fireEvent.click(dialog().getByRole('radio', { name: 'OpenAI' }));
    expect(dialog().getByRole('textbox', { name: /^Name/ })).toHaveValue('claude');
    expect(dialog().getByLabelText(/^API key/)).toHaveValue('');
    fireEvent.click(dialog().getByRole('button', { name: 'Save provider' }));
    expect(
      await dialog().findByText('Enter a new API key: the saved key belongs to the previous type.')
    ).toBeInTheDocument();
    expect(updateModelCatalog).not.toHaveBeenCalled();

    type(dialog().getByLabelText(/^API key/), TYPED_KEY);
    fireEvent.click(dialog().getByRole('button', { name: 'Save provider' }));

    await waitFor(() => expect(updateModelCatalog).toHaveBeenCalled());
    const [, spec, secure] = lastUpdate();
    expect(spec.providers.claude).toEqual({ type: 'openai' });
    expect(secure).toEqual({ claude: { create: TYPED_KEY } });
  });

  test('removes an unused provider and its key after confirming, and keeps used ones', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    expect(row('claude').getByRole('button', { name: "claude can't be removed while models use it" })).toBeDisabled();
    fireEvent.click(row('spare').getByRole('button', { name: 'Remove spare' }));
    expect(updateModelCatalog).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() => expect(screen.queryByRole('listitem', { name: 'spare' })).not.toBeInTheDocument());
    const [, spec, secure] = lastUpdate();
    expect(Object.keys(spec.providers)).toEqual(['claude', 'gpt']);
    expect(secure).toEqual({ spare: { remove: true } });
  });

  test('adds a model with its primary and ordered fallbacks', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(screen.getByRole('button', { name: 'Add model' }));
    type(dialog().getByRole('textbox', { name: /^Model ID/ }), 'fast');
    type(dialog().getByRole('textbox', { name: /^Display name/ }), 'Fast');
    chooseOption(dialog().getByRole('combobox', { name: 'Primary provider' }), 'claude');
    type(dialog().getByRole('textbox', { name: 'Primary model' }), 'claude-haiku-4-5');
    fireEvent.click(dialog().getByRole('button', { name: 'Add fallback' }));
    fireEvent.click(dialog().getByRole('button', { name: 'Add fallback' }));
    chooseOption(dialog().getByRole('combobox', { name: 'Fallback 1 provider' }), 'gpt');
    type(dialog().getByRole('textbox', { name: 'Fallback 1 model' }), 'gpt-5-mini');
    chooseOption(dialog().getByRole('combobox', { name: 'Fallback 2 provider' }), 'spare');
    type(dialog().getByRole('textbox', { name: 'Fallback 2 model' }), 'gpt-5');
    fireEvent.click(dialog().getByRole('button', { name: 'Move fallback 2 up' }));
    fireEvent.click(dialog().getByRole('button', { name: 'Add model' }));

    expect(await screen.findByRole('listitem', { name: 'fast' })).toBeInTheDocument();
    expect(lastUpdate()[1].models.fast).toEqual({
      name: 'Fast',
      primary: { provider: 'claude', model: 'claude-haiku-4-5' },
      fallback: [
        { provider: 'spare', model: 'gpt-5' },
        { provider: 'gpt', model: 'gpt-5-mini' },
      ],
    });
    expect(lastUpdate()[2]).toEqual({});
  });

  test('edits a model and keeps the fields the page does not edit', async () => {
    const withSkills = {
      ...stored,
      spec: {
        ...stored.spec,
        models: { ...stored.spec.models, balanced: { ...stored.spec.models.balanced, skills: ['x'] } },
      },
    } as ModelCatalog;
    jest.mocked(getModelCatalog).mockResolvedValue(withSkills);
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(screen.getByRole('button', { name: 'Edit balanced' }));
    expect(dialog().getByRole('textbox', { name: /^Model ID/ })).toBeDisabled();
    type(dialog().getByRole('textbox', { name: /^Display name/ }), 'Renamed');
    fireEvent.click(dialog().getByRole('button', { name: 'Save model' }));

    expect(await screen.findByText('Renamed')).toBeInTheDocument();
    expect(lastUpdate()[1].models.balanced).toEqual({ ...withSkills.spec.models.balanced, name: 'Renamed' });
  });

  test('removes a model after confirming, but not the last one', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(row('plain').getByRole('button', { name: 'Remove plain' }));
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() => expect(screen.queryByRole('listitem', { name: 'plain' })).not.toBeInTheDocument());
    expect(Object.keys(lastUpdate()[1].models)).toEqual(['balanced']);
    expect(row('balanced').getByRole('button', { name: 'A catalog needs at least one model' })).toBeDisabled();
  });

  test('shows a refused change as returned in its dialog and keeps the input', async () => {
    const message = 'Operation cannot be fulfilled on modelcatalogs "team": the object has been modified';
    jest.mocked(updateModelCatalog).mockRejectedValue({ status: 409, data: { kind: 'Status', message } });
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(screen.getByRole('button', { name: 'Edit claude' }));
    fireEvent.click(dialog().getByRole('button', { name: 'Reset' }));
    type(dialog().getByLabelText(/^API key/), TYPED_KEY);
    fireEvent.click(dialog().getByRole('button', { name: 'Save provider' }));

    expect(await dialog().findByText(message)).toBeInTheDocument();
    expect(dialog().getByLabelText(/^API key/)).toHaveValue(TYPED_KEY);
    expect(document.body).not.toHaveTextContent(TYPED_KEY);
  });

  test('shows a refused removal as returned on the page', async () => {
    jest.mocked(updateModelCatalog).mockRejectedValue({ status: 403, data: { message: 'forbidden: catalogwriter' } });
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(row('plain').getByRole('button', { name: 'Remove plain' }));
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));

    expect(await screen.findByText('forbidden: catalogwriter')).toBeInTheDocument();
    expect(screen.getByRole('listitem', { name: 'plain' })).toBeInTheDocument();
  });

  test('deletes the catalog after confirming and returns to the list', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(screen.getByRole('button', { name: 'Delete catalog' }));
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

    expect(await screen.findByText('catalog list')).toBeInTheDocument();
    expect(deleteModelCatalog).toHaveBeenCalledWith('team');
  });

  test('requests nothing without the read action', () => {
    signInAs(OrgRole.Admin, []);
    renderPage();

    expect(screen.getByText("You can't view model catalogs")).toBeInTheDocument();
    expect(getModelCatalog).not.toHaveBeenCalled();
  });

  test('shows a failed load as returned', async () => {
    jest
      .mocked(getModelCatalog)
      .mockRejectedValue({ status: 404, data: { message: 'modelcatalogs "team" not found' } });
    signInAs(OrgRole.Viewer, READ_ONLY);
    renderPage();

    expect(await screen.findByText('modelcatalogs "team" not found')).toBeInTheDocument();
  });
});
