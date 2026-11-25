import React from 'react';
import { Link, MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom';
import { OrgRole } from '@grafana/data';
import { PluginPageProps } from '@grafana/runtime';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import CatalogPage from './CatalogPage';
import {
  CatalogHealth,
  checkModelCatalogHealth,
  deleteModelCatalog,
  getModelCatalog,
  listProviderModels,
  ProviderModelListing,
  SecureChanges,
  updateModelCatalog,
} from '../api/modelCatalogs';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { Spec } from '../generated/modelcatalog/v1alpha1/types.spec.gen';
import { CatalogActions } from '../permissions';
import { signInAs } from '../testUtils';

jest.mock('../api/modelCatalogs', () => ({
  ...jest.requireActual('../api/modelCatalogs'),
  getModelCatalog: jest.fn(),
  checkModelCatalogHealth: jest.fn(),
  listProviderModels: jest.fn(),
  updateModelCatalog: jest.fn(),
  deleteModelCatalog: jest.fn(),
}));

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  config: { appUrl: 'https://grafana.example/', namespace: 'stacks-123' },
  PluginPage: ({ subTitle, actions, children }: PluginPageProps) => (
    <>
      {subTitle}
      <div role="group" aria-label="Catalog actions">
        {actions}
      </div>
      {children}
    </>
  ),
}));

const APP = '/a/grafana-aigateway-app';
const TYPED_KEY = 'typed-provider-key';
const READ_ONLY = [CatalogActions.Read];
const SUCCESS_BADGES = /^(Key saved|Key accepted|Model listed)$/;
const ALL = Object.values(CatalogActions);

const stored: ModelCatalog = {
  apiVersion: 'aigateway.ext.grafana.app/v1alpha1',
  kind: 'ModelCatalog',
  metadata: { name: 'team', namespace: 'stacks-123', resourceVersion: '7' },
  spec: {
    providers: { anthropic: { type: 'anthropic' }, openai: { type: 'openai' }, spare: { type: 'openai' } },
    models: {
      balanced: {
        name: 'Balanced',
        aliases: ['default'],
        primary: { provider: 'anthropic', model: 'claude-sonnet-4-6' },
        fallback: [{ provider: 'openai', model: 'gpt-5' }],
      },
      'gpt-5': { name: 'gpt-5', primary: { provider: 'openai', model: 'gpt-5' } },
    },
  },
  secure: {
    anthropic: { name: 'inline-anthropic' },
    openai: { name: 'inline-openai' },
    spare: { name: 'inline-spare' },
  },
};

const listing: CatalogHealth = {
  providers: { anthropic: 'ok', openai: 'invalidKey', spare: 'unreachable' },
  models: { balanced: { primary: 'ok', fallback: ['notChecked'] }, 'gpt-5': { primary: 'notChecked', fallback: [] } },
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

function BackButton() {
  const navigate = useNavigate();
  return (
    <button type="button" onClick={() => navigate(-1)}>
      Back
    </button>
  );
}

function renderPage(withNavigation = false, arrival?: 'create' | 'reload') {
  return render(
    <MemoryRouter
      initialEntries={[
        {
          pathname: arrival === 'create' ? `${APP}/catalogs` : `${APP}/catalogs/edit/team`,
          state: arrival === 'reload' ? { checkHealthAfterCreate: true } : null,
        },
      ]}
    >
      {withNavigation && (
        <>
          <Link to={`${APP}/catalogs`}>Catalog list</Link>
          <Link to={`${APP}/catalogs/edit/team`}>Team catalog</Link>
          <Link to={`${APP}/catalogs/edit/team`} state={{ checkHealthAfterCreate: true }}>
            Create arrival
          </Link>
          <BackButton />
        </>
      )}
      <Routes>
        <Route path={`${APP}/catalogs`} element={<div>catalog list</div>} />
        <Route path={`${APP}/catalogs/edit/:name`} element={<CatalogPage />} />
      </Routes>
    </MemoryRouter>
  );
}

async function openPage() {
  renderPage();
  await screen.findByRole('listitem', { name: 'balanced' });
}

function card(provider: string) {
  return within(screen.getByRole('region', { name: provider }));
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
    jest.mocked(checkModelCatalogHealth).mockReset().mockResolvedValue(listing);
    jest
      .mocked(listProviderModels)
      .mockReset()
      .mockResolvedValue({ status: 'ok', modelIds: ['claude-listed', 'claude-other'] });
    jest
      .mocked(updateModelCatalog)
      .mockReset()
      .mockImplementation((catalog, spec, secure) => Promise.resolve(updated(catalog, spec, secure)));
    jest.mocked(deleteModelCatalog).mockReset().mockResolvedValue();
  });

  test('uses the model ID field to fetch, search and choose, with Add model as the only write', async () => {
    signInAs(OrgRole.Viewer, ALL);
    await openPage();
    const provider = card('anthropic');
    const field = provider.getByRole('combobox', { name: 'Add a model to anthropic' });
    fireEvent.focus(field);
    expect(field).toHaveAttribute('aria-expanded', 'false');
    expect(listProviderModels).not.toHaveBeenCalled();

    fireEvent.click(field);
    await provider.findByRole('option', { name: 'claude-listed' });
    expect(listProviderModels).toHaveBeenCalledWith('team', 'anthropic');
    type(field, 'listed');
    expect(provider.queryByRole('option', { name: 'claude-other' })).not.toBeInTheDocument();
    fireEvent.keyDown(field, { key: 'ArrowDown' });
    expect(field).toHaveAttribute('aria-activedescendant', provider.getByRole('option', { name: 'claude-listed' }).id);
    expect(fireEvent.keyDown(field, { key: 'Enter' })).toBe(false);
    expect(field).toHaveValue('claude-listed');
    expect(field).toHaveAttribute('aria-expanded', 'false');
    expect(listProviderModels).toHaveBeenCalledTimes(1);
    expect(updateModelCatalog).not.toHaveBeenCalled();
    fireEvent.click(provider.getByRole('button', { name: 'Add model' }));
    await provider.findByRole('listitem', { name: 'claude-listed' });
    expect(lastUpdate()[1].models['claude-listed']?.primary).toEqual({ provider: 'anthropic', model: 'claude-listed' });
  });

  test.each(['Tab', 'Escape'])('%s closes suggestions without replacing the typed ID', async (key) => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();
    const provider = card('anthropic');
    const field = provider.getByRole('combobox', { name: 'Add a model to anthropic' });
    type(field, 'claude');
    await provider.findByRole('option', { name: 'claude-listed' });
    fireEvent.keyDown(field, { key: 'ArrowDown' });
    expect(field).toHaveAttribute('aria-activedescendant', provider.getByRole('option', { name: 'claude-listed' }).id);
    fireEvent.keyDown(field, { key });
    if (key === 'Tab') {
      fireEvent.blur(field, { relatedTarget: provider.getByRole('button', { name: 'Add model' }) });
    }
    expect(field).toHaveValue('claude');
    expect(field).toHaveAttribute('aria-expanded', 'false');
    expect(updateModelCatalog).not.toHaveBeenCalled();
  });

  test.each([
    [CatalogActions.Read, CatalogActions.Update],
    [CatalogActions.Read, CatalogActions.ListProviderModels],
  ])('does not offer listings with only %s and %s', async (read, action) => {
    signInAs(OrgRole.Admin, [read, action]);
    await openPage();
    expect(screen.queryByRole('combobox', { name: 'Add a model to anthropic' })).not.toBeInTheDocument();
    expect(listProviderModels).not.toHaveBeenCalled();
  });

  test.each([
    [{ status: 'ok', modelIds: [] }, 'The provider returned no models.'],
    [{ status: 'ok', modelIds: ['claude-listed'] }, 'No matching models.'],
    [{ status: 'partial', modelIds: ['claude-listed'] }, 'The provider listing is incomplete.'],
    [{ status: 'invalidKey', modelIds: [] }, 'The provider rejected the saved key.'],
    [undefined, 'Could not list models.'],
  ] satisfies Array<[ProviderModelListing | undefined, string]>)(
    'can add a custom ID with listing %j',
    async (result, message) => {
      signInAs(OrgRole.Admin, ALL);
      if (result) {
        jest.mocked(listProviderModels).mockResolvedValueOnce(result);
      } else {
        jest.mocked(listProviderModels).mockRejectedValueOnce(new Error('raw-provider-error-sentinel'));
      }
      await openPage();
      const provider = card('anthropic');
      const field = provider.getByRole('combobox', { name: 'Add a model to anthropic' });
      type(field, 'claude-custom');
      expect(provider.getByRole('status')).toHaveTextContent('Loading provider models…');
      await provider.findByText(message, { exact: false });
      expect(screen.queryByText('raw-provider-error-sentinel')).not.toBeInTheDocument();
      expect(field).toHaveValue('claude-custom');
      fireEvent.click(provider.getByRole('button', { name: 'Add model' }));
      await provider.findByRole('listitem', { name: 'claude-custom' });
    }
  );

  test('reopens a pending listing without another request and fetches again after it settles', async () => {
    signInAs(OrgRole.Admin, ALL);
    let resolve!: (value: ProviderModelListing) => void;
    jest.mocked(listProviderModels).mockReturnValueOnce(
      new Promise((finish) => {
        resolve = finish;
      })
    );
    await openPage();
    const provider = card('anthropic');
    const field = provider.getByRole('combobox', { name: 'Add a model to anthropic' });
    fireEvent.click(field);
    fireEvent.keyDown(field, { key: 'Escape' });
    fireEvent.click(field);
    expect(provider.getByRole('status')).toHaveTextContent('Loading provider models…');
    expect(listProviderModels).toHaveBeenCalledTimes(1);
    await act(() => resolve({ status: 'ok', modelIds: ['reused-model'] }));
    expect(provider.getByRole('option', { name: 'reused-model' })).toBeInTheDocument();

    fireEvent.keyDown(field, { key: 'Escape' });
    fireEvent.click(field);
    expect(await provider.findByRole('option', { name: 'claude-listed' })).toBeInTheDocument();
    expect(listProviderModels).toHaveBeenCalledTimes(2);
  });

  test('does not reuse a pending listing after replacing the key', async () => {
    signInAs(OrgRole.Admin, ALL);
    let resolve!: (value: ProviderModelListing) => void;
    const pending = new Promise<ProviderModelListing>((finish) => {
      resolve = finish;
    });
    jest.mocked(listProviderModels).mockReturnValueOnce(pending);
    await openPage();
    const provider = card('anthropic');
    const fieldName = { name: 'Add a model to anthropic' };
    fireEvent.click(provider.getByRole('combobox', fieldName));
    fireEvent.click(provider.getByRole('button', { name: 'Replace key' }));
    type(dialog().getByLabelText(/^New API key/), TYPED_KEY);
    fireEvent.click(dialog().getByRole('button', { name: 'Save key' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());

    fireEvent.click(provider.getByRole('combobox', fieldName));
    expect(await provider.findByRole('option', { name: 'claude-listed' })).toBeInTheDocument();
    expect(listProviderModels).toHaveBeenCalledTimes(2);
    await act(() => resolve({ status: 'ok', modelIds: ['old-key-model'] }));
    expect(provider.queryByRole('option', { name: 'old-key-model' })).not.toBeInTheDocument();
  });

  test('shows the busy message for both listing and health 429s', async () => {
    signInAs(OrgRole.Admin, ALL);
    const busy = { status: 429, data: { message: 'untrusted-busy-detail' } };
    jest.mocked(listProviderModels).mockRejectedValueOnce(busy);
    jest.mocked(checkModelCatalogHealth).mockRejectedValueOnce(busy);
    await openPage();
    const provider = card('anthropic');
    fireEvent.click(provider.getByRole('combobox', { name: 'Add a model to anthropic' }));
    const message = 'Another check is running for this catalog. Try again in a moment.';
    expect(await provider.findByText(message)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Check health' }));
    const alert = await screen.findByText('Catalog health check failed');
    expect(alert.closest('[role="alert"]')).toHaveTextContent(message);
    expect(document.body).not.toHaveTextContent('untrusted-busy-detail');
  });

  test.each(['escape', 'save', 'navigate'])('ignores a listing that finishes after %s', async (action) => {
    signInAs(OrgRole.Admin, ALL);
    let resolve!: (value: ProviderModelListing) => void;
    jest.mocked(listProviderModels).mockReturnValueOnce(
      new Promise((finish) => {
        resolve = finish;
      })
    );
    renderPage(true);
    await screen.findByRole('listitem', { name: 'balanced' });
    const provider = card('anthropic');
    const field = provider.getByRole('combobox', { name: 'Add a model to anthropic' });
    type(field, 'claude-custom');
    expect(provider.getByRole('status')).toHaveTextContent('Loading provider models…');
    switch (action) {
      case 'escape':
        fireEvent.keyDown(field, { key: 'Escape' });
        break;
      case 'save':
        fireEvent.click(provider.getByRole('button', { name: 'Add model' }));
        await provider.findByRole('listitem', { name: 'claude-custom' });
        break;
      case 'navigate':
        fireEvent.click(screen.getByRole('link', { name: 'Catalog list' }));
        fireEvent.click(screen.getByRole('link', { name: 'Team catalog' }));
        await screen.findByRole('listitem', { name: 'balanced' });
        break;
    }
    await act(() => resolve({ status: 'ok', modelIds: ['claude-custom-stale'] }));
    expect(card('anthropic').getByRole('combobox', { name: 'Add a model to anthropic' })).toHaveAttribute(
      'aria-expanded',
      'false'
    );
    expect(screen.queryByRole('option', { name: 'claude-custom-stale' })).not.toBeInTheDocument();
  });

  test('checks health once on create arrival and offers browsing before the first model', async () => {
    jest.mocked(getModelCatalog).mockResolvedValue({ ...stored, spec: { ...stored.spec, models: {} } });
    jest.mocked(checkModelCatalogHealth).mockResolvedValue({
      providers: { anthropic: 'ok', openai: 'ok', spare: 'ok' },
      models: {},
    });
    signInAs(OrgRole.Admin, ALL);
    renderPage(true, 'create');
    fireEvent.click(screen.getByRole('link', { name: 'Create arrival' }));

    const provider = within(await screen.findByRole('region', { name: 'anthropic' }));
    expect(await screen.findByText('No problems found')).toBeInTheDocument();
    expect(checkModelCatalogHealth).toHaveBeenCalledTimes(1);
    expect(checkModelCatalogHealth).toHaveBeenCalledWith('team');
    expect(provider.queryByText(SUCCESS_BADGES)).not.toBeInTheDocument();
    const field = provider.getByRole('combobox', { name: 'Add a model to anthropic' });
    fireEvent.focus(field);
    expect(listProviderModels).not.toHaveBeenCalled();
    fireEvent.click(field);
    expect(await provider.findByRole('option', { name: 'claude-listed' })).toBeInTheDocument();
    expect(listProviderModels).toHaveBeenCalledWith('team', 'anthropic');

    fireEvent.click(screen.getByRole('link', { name: 'Catalog list' }));
    fireEvent.click(screen.getByRole('button', { name: 'Back' }));
    await screen.findByRole('region', { name: 'anthropic' });
    expect(checkModelCatalogHealth).toHaveBeenCalledTimes(1);
    expect(screen.queryByText('No problems found')).not.toBeInTheDocument();
  });

  test('does not run arrival health without its permission', async () => {
    signInAs(
      OrgRole.Admin,
      ALL.filter((action) => action !== CatalogActions.CheckHealth)
    );
    renderPage(true, 'create');
    fireEvent.click(screen.getByRole('link', { name: 'Create arrival' }));
    await screen.findByRole('listitem', { name: 'balanced' });
    expect(checkModelCatalogHealth).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'Check health' })).not.toBeInTheDocument();
  });

  test('does not run arrival health from a refreshed history entry', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderPage(false, 'reload');
    await screen.findByRole('listitem', { name: 'balanced' });
    expect(checkModelCatalogHealth).not.toHaveBeenCalled();
  });

  test('does not replay a create arrival left before the catalog loaded', async () => {
    signInAs(OrgRole.Admin, ALL);
    let finish!: (catalog: ModelCatalog) => void;
    jest.mocked(getModelCatalog).mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      })
    );
    renderPage(true, 'create');
    fireEvent.click(screen.getByRole('link', { name: 'Create arrival' }));
    expect(getModelCatalog).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('link', { name: 'Catalog list' }));
    fireEvent.click(screen.getByRole('button', { name: 'Back' }));
    await screen.findByRole('listitem', { name: 'balanced' });
    await act(() => finish(stored));
    expect(checkModelCatalogHealth).not.toHaveBeenCalled();
  });

  test('no health action is offered for an invalid catalog missing providers', async () => {
    signInAs(OrgRole.Admin, ALL);
    jest.mocked(getModelCatalog).mockResolvedValue({ ...stored, spec: { providers: {}, models: {} } });
    renderPage(true, 'create');
    fireEvent.click(screen.getByRole('link', { name: 'Create arrival' }));
    await screen.findByRole('button', { name: 'Connect a provider' });
    expect(checkModelCatalogHealth).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'Check health' })).not.toBeInTheDocument();
  });

  test('connecting another provider does not reuse a key after switching types', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();
    fireEvent.click(screen.getByRole('button', { name: 'Connect a provider' }));
    fireEvent.click(dialog().getByRole('radio', { name: 'Anthropic' }));
    type(dialog().getByLabelText(/^API key/), TYPED_KEY);
    fireEvent.click(dialog().getByRole('radio', { name: 'OpenAI' }));
    fireEvent.click(dialog().getByRole('button', { name: 'Connect' }));
    expect(await dialog().findByText('Enter the API key.')).toBeInTheDocument();
    expect(updateModelCatalog).not.toHaveBeenCalled();
  });

  test('checks only on request and shows badges only for listing problems', async () => {
    signInAs(OrgRole.Viewer, [...READ_ONLY, CatalogActions.CheckHealth]);
    await openPage();
    expect(checkModelCatalogHealth).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Check health' }));
    expect(screen.getByRole('button', { name: 'Checking health…' })).toHaveAttribute('aria-disabled', 'true');
    expect(await card('openai').findByText('Key rejected')).toBeInTheDocument();
    expect(checkModelCatalogHealth).toHaveBeenCalledWith('team');
    expect(card('spare').getByText('Provider unreachable')).toBeInTheDocument();
    expect(card('openai').getByText('Not checked')).toBeInTheDocument();
    expect(within(screen.getByRole('region', { name: 'Providers' })).getByRole('status')).toHaveTextContent(
      '4 problems'
    );
    expect(screen.queryByText(SUCCESS_BADGES)).not.toBeInTheDocument();
    expect(screen.queryByText('No problems found')).not.toBeInTheDocument();
    fireEvent.click(card('anthropic').getByRole('button', { name: 'balanced' }));
    expect(dialog().getByText('Not checked')).toBeInTheDocument();
  });

  test('ignores a repeated click and shows the first check result', async () => {
    signInAs(OrgRole.Admin, ALL);
    let finish!: (result: CatalogHealth) => void;
    const pending = new Promise<CatalogHealth>((resolve) => {
      finish = resolve;
    });
    jest.mocked(checkModelCatalogHealth).mockReturnValue(pending);
    await openPage();

    const check = screen.getByRole('button', { name: 'Check health' });
    act(() => {
      fireEvent.click(check);
      fireEvent.click(check);
    });
    expect(checkModelCatalogHealth).toHaveBeenCalledTimes(1);

    await act(async () => finish(listing));
    expect(card('openai').getByText('Key rejected')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Check health' })).toBeEnabled();
  });

  test('shows one quiet confirmation when all providers and targets pass', async () => {
    signInAs(OrgRole.Admin, ALL);
    jest.mocked(checkModelCatalogHealth).mockResolvedValueOnce({
      providers: { anthropic: 'ok', openai: 'ok', spare: 'ok' },
      models: { balanced: { primary: 'ok', fallback: ['ok'] }, 'gpt-5': { primary: 'ok', fallback: [] } },
    });
    await openPage();
    expect(screen.queryByText('No problems found')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Check health' }));
    const providers = within(screen.getByRole('region', { name: 'Providers' }));
    expect(await providers.findByText('No problems found')).toBeInTheDocument();
    expect(providers.getByRole('status')).toHaveTextContent('No problems found');
    expect(screen.queryByText(SUCCESS_BADGES)).not.toBeInTheDocument();
    const actions = within(screen.getByRole('group', { name: 'Catalog actions' }));
    expect(actions.queryByRole('status')).not.toBeInTheDocument();
    expect(actions.getAllByRole('button').map((button) => button.textContent)).toEqual([
      'Check health',
      'Delete catalog',
    ]);
    fireEvent.click(card('anthropic').getByRole('button', { name: 'balanced' }));
    expect(dialog().queryByText(SUCCESS_BADGES)).not.toBeInTheDocument();
    fireEvent.click(dialog().getByRole('button', { name: 'Close' }));

    jest.mocked(checkModelCatalogHealth).mockReturnValueOnce(new Promise(() => {}));
    fireEvent.click(actions.getByRole('button', { name: 'Check health' }));
    expect(providers.getByRole('status')).toBeEmptyDOMElement();
  });

  test('a writer without the explicit health permission cannot check', async () => {
    signInAs(
      OrgRole.Admin,
      ALL.filter((action) => action !== CatalogActions.CheckHealth)
    );
    await openPage();
    expect(screen.queryByRole('button', { name: 'Check health' })).not.toBeInTheDocument();
    expect(checkModelCatalogHealth).not.toHaveBeenCalled();
  });

  test('ignores a pending check after a successful edit', async () => {
    signInAs(OrgRole.Admin, ALL);
    let finish!: (result: CatalogHealth) => void;
    const pending = new Promise<CatalogHealth>((resolve) => {
      finish = resolve;
    });
    jest.mocked(checkModelCatalogHealth).mockReturnValueOnce(pending);
    await openPage();
    fireEvent.click(screen.getByRole('button', { name: 'Check health' }));
    fireEvent.click(card('anthropic').getByRole('button', { name: 'balanced' }));
    type(dialog().getByRole('textbox', { name: /^Display name/ }), 'Edited');
    fireEvent.click(dialog().getByRole('button', { name: 'Save model' }));
    expect(await card('anthropic').findByText('Edited')).toBeInTheDocument();
    await act(async () => finish(listing));
    expect(card('openai').queryByText('Key rejected')).not.toBeInTheDocument();
    expect(within(screen.getByRole('region', { name: 'Providers' })).getByRole('status')).toBeEmptyDOMElement();
    expect(screen.getByRole('button', { name: 'Check health' })).toBeEnabled();
  });

  test('ignores a pending health result after leaving and reopening the catalog', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderPage(true);
    const first = await screen.findByRole('button', { name: 'Check health' });
    let finish!: (result: CatalogHealth) => void;
    jest.mocked(checkModelCatalogHealth).mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      })
    );
    fireEvent.click(first);
    fireEvent.click(screen.getByRole('link', { name: 'Catalog list' }));
    expect(screen.queryByRole('button', { name: 'Checking health…' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('link', { name: 'Team catalog' }));
    await act(async () => finish(listing));
    expect(await screen.findByRole('button', { name: 'Check health' })).toBeEnabled();
    expect(card('openai').queryByText('Key rejected')).not.toBeInTheDocument();
    expect(checkModelCatalogHealth).toHaveBeenCalledTimes(1);
  });

  test('uses fixed health failure text without displaying the underlying error', async () => {
    signInAs(OrgRole.Admin, ALL);
    jest.mocked(checkModelCatalogHealth).mockRejectedValueOnce(new Error('untrusted-provider-message'));
    await openPage();
    fireEvent.click(screen.getByRole('button', { name: 'Check health' }));
    expect(await screen.findByText('The catalog health check could not run. Try again.')).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent('untrusted-provider-message');
  });

  test('shows each provider and the models it serves, without key badges or secure names', async () => {
    signInAs(OrgRole.Viewer, READ_ONLY);
    await openPage();

    expect(
      screen.getByText(
        'https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123/modelcatalogs/team'
      )
    ).toBeInTheDocument();
    expect(card('anthropic').getByRole('heading', { name: 'Anthropic' })).toBeInTheDocument();
    expect(screen.queryByText('Key saved')).not.toBeInTheDocument();
    expect(card('anthropic').getByRole('listitem', { name: 'balanced' })).toBeInTheDocument();
    expect(card('anthropic').getByText('Balanced')).toBeInTheDocument();
    expect(card('anthropic').getByText('serves claude-sonnet-4-6')).toBeInTheDocument();
    expect(card('anthropic').getByText('Falls back to gpt-5 on openai')).toBeInTheDocument();
    expect(card('anthropic').getByText('Also requested as default')).toBeInTheDocument();
    expect(card('openai').getByRole('listitem', { name: 'gpt-5' })).toBeInTheDocument();
    expect(card('spare').getByText('spare')).toBeInTheDocument();
    expect(card('spare').getByText('No models yet.')).toBeInTheDocument();
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

    expect(screen.queryByRole('button', { name: 'Connect a provider' }) !== null).toBe(update);
    expect(screen.queryByRole('textbox', { name: 'Add a model to anthropic' }) !== null).toBe(update);
    expect(screen.queryAllByRole('button', { name: 'Replace key' }).length > 0).toBe(update);
    expect(screen.queryByRole('button', { name: 'Remove gpt-5' }) !== null).toBe(update);
    expect(screen.queryByText('Read-only') === null).toBe(update);
    expect(screen.queryByRole('button', { name: 'Delete catalog' }) !== null).toBe(remove);
  });

  test('adds a provider model in one step, under its own ID', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    type(card('anthropic').getByRole('combobox', { name: 'Add a model to anthropic' }), 'claude-haiku-4-5');
    fireEvent.click(card('anthropic').getByRole('button', { name: 'Add model' }));

    expect(await card('anthropic').findByRole('listitem', { name: 'claude-haiku-4-5' })).toBeInTheDocument();
    const [catalog, spec, secure] = lastUpdate();
    expect(catalog).toBe(stored);
    expect(spec.models['claude-haiku-4-5']).toEqual({
      name: 'claude-haiku-4-5',
      primary: { provider: 'anthropic', model: 'claude-haiku-4-5' },
    });
    expect(secure).toEqual({});
    expect(card('anthropic').getByRole('combobox', { name: 'Add a model to anthropic' })).toHaveValue('');
  });

  test('shows a format error when the quick-add field is left and clears it as the value is fixed', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();
    const field = card('anthropic').getByRole('combobox', { name: 'Add a model to anthropic' });
    const formatError = /Applications request the model by this ID\./;

    type(field, 'bad id');
    expect(card('anthropic').queryByText(formatError)).not.toBeInTheDocument();
    fireEvent.blur(field);
    expect(card('anthropic').getByText(formatError)).toBeInTheDocument();
    type(field, 'good-id');
    expect(card('anthropic').queryByText(formatError)).not.toBeInTheDocument();

    type(field, '');
    fireEvent.blur(field);
    expect(card('anthropic').queryByText("Enter the provider's model ID.")).not.toBeInTheDocument();
    fireEvent.click(card('anthropic').getByRole('button', { name: 'Add model' }));
    expect(await card('anthropic').findByText("Enter the provider's model ID.")).toBeInTheDocument();
    expect(updateModelCatalog).not.toHaveBeenCalled();
  });

  test('clears the quick-add field and its errors after a model is added', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();
    const field = card('anthropic').getByRole('combobox', { name: 'Add a model to anthropic' });

    fireEvent.click(card('anthropic').getByRole('button', { name: 'Add model' }));
    expect(await card('anthropic').findByText("Enter the provider's model ID.")).toBeInTheDocument();
    type(field, 'claude-haiku-4-5');
    fireEvent.click(card('anthropic').getByRole('button', { name: 'Add model' }));

    expect(await card('anthropic').findByRole('listitem', { name: 'claude-haiku-4-5' })).toBeInTheDocument();
    expect(card('anthropic').getByRole('combobox', { name: 'Add a model to anthropic' })).toHaveValue('');
    expect(card('anthropic').queryByText("Enter the provider's model ID.")).not.toBeInTheDocument();
  });

  test('refuses to add a model under an ID the catalog has, without a request', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    type(card('anthropic').getByRole('combobox', { name: 'Add a model to anthropic' }), 'gpt-5');
    fireEvent.click(card('anthropic').getByRole('button', { name: 'Add model' }));

    expect(await card('anthropic').findByText(/already has a model with ID "gpt-5"/)).toBeInTheDocument();
    expect(updateModelCatalog).not.toHaveBeenCalled();
  });

  test('connects a provider named after its type, with its key as secure.create', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(screen.getByRole('button', { name: 'Connect a provider' }));
    fireEvent.click(dialog().getByRole('radio', { name: 'OpenAI' }));
    expect(dialog().getByRole('textbox', { name: /^Account name/ })).toHaveValue('openai-2');
    type(dialog().getByLabelText(/^API key/), TYPED_KEY);
    fireEvent.click(dialog().getByRole('button', { name: 'Connect' }));

    expect(await screen.findByRole('region', { name: 'openai-2' })).toBeInTheDocument();
    const [, spec, secure] = lastUpdate();
    expect(spec.providers['openai-2']).toEqual({ type: 'openai' });
    expect(secure).toEqual({ 'openai-2': { create: TYPED_KEY } });
    expect(card('openai-2').getByRole('button', { name: 'Replace key' })).toBeInTheDocument();
    expect(document.body.innerHTML).not.toContain(TYPED_KEY);
  });

  test('asks for no account name when the type is free', async () => {
    signInAs(OrgRole.Admin, ALL);
    jest.mocked(getModelCatalog).mockResolvedValue({
      ...stored,
      spec: { providers: { openai: { type: 'openai' } }, models: { 'gpt-5': stored.spec.models['gpt-5'] } },
    });
    renderPage();
    await screen.findByRole('listitem', { name: 'gpt-5' });

    fireEvent.click(screen.getByRole('button', { name: 'Connect a provider' }));
    expect(dialog().getByRole('radio', { name: 'Anthropic' })).toBeChecked();
    expect(dialog().queryByRole('textbox', { name: /^Account name/ })).not.toBeInTheDocument();
    fireEvent.click(dialog().getByRole('button', { name: 'Connect' }));
    expect(await dialog().findByText('Enter the API key.')).toBeInTheDocument();
    expect(updateModelCatalog).not.toHaveBeenCalled();
  });

  test.each([
    [undefined, "Anthropic's native endpoint"],
    ['https://gateway.example/team', 'https://gateway.example/team'],
  ])('shows the destination %s and sends only the replacement key', async (baseURL, destination) => {
    const catalog = {
      ...stored,
      spec: { ...stored.spec, providers: { ...stored.spec.providers, anthropic: { type: 'anthropic', baseURL } } },
    };
    jest.mocked(getModelCatalog).mockResolvedValue(catalog);
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(card('anthropic').getByRole('button', { name: 'Replace key' }));
    expect(dialog().getByText(`Use a key for ${destination}.`, { exact: false })).toBeInTheDocument();
    type(dialog().getByLabelText(/^New API key/), TYPED_KEY);
    fireEvent.click(dialog().getByRole('button', { name: 'Save key' }));

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    const [, spec, secure] = lastUpdate();
    expect(spec).toEqual(catalog.spec);
    expect(secure).toEqual({ anthropic: { create: TYPED_KEY } });
  });

  test('removes a provider with its key and models after confirming, but not one others fall back to', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    expect(
      card('openai').getByRole('button', { name: "openai can't be removed while balanced falls back to it" })
    ).toBeDisabled();
    fireEvent.click(card('anthropic').getByRole('button', { name: 'Remove anthropic' }));
    expect(screen.getByText(/its models are removed: balanced/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() => expect(screen.queryByRole('region', { name: 'anthropic' })).not.toBeInTheDocument());
    const [, spec, secure] = lastUpdate();
    expect(spec).toEqual({
      providers: { openai: stored.spec.providers.openai, spare: stored.spec.providers.spare },
      models: { 'gpt-5': stored.spec.models['gpt-5'] },
    });
    expect(secure).toEqual({ anthropic: { remove: true } });
  });

  test('closes the key dialog if a pending removal deletes its provider', async () => {
    let finish!: (catalog: ModelCatalog) => void;
    jest.mocked(updateModelCatalog).mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve;
      })
    );
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(card('spare').getByRole('button', { name: 'Remove spare' }));
    fireEvent.click(dialog().getByRole('button', { name: 'Remove' }));
    await waitFor(() => expect(updateModelCatalog).toHaveBeenCalledTimes(1));
    fireEvent.click(card('spare').getByRole('button', { name: 'Replace key' }));
    expect(dialog().getByLabelText(/^New API key/)).toBeInTheDocument();

    const [catalog, spec, secure] = lastUpdate();
    await act(() => finish(updated(catalog, spec, secure)));
    expect(screen.queryByRole('region', { name: 'spare' })).not.toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.getByRole('listitem', { name: 'balanced' })).toBeInTheDocument();
  });

  test('renames a model and edits its details in the drawer, keeping fields the page does not edit', async () => {
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

    fireEvent.click(card('anthropic').getByRole('button', { name: 'balanced' }));
    type(dialog().getByRole('textbox', { name: /^Model ID/ }), 'general');
    type(dialog().getByRole('textbox', { name: /^Display name/ }), 'General');
    fireEvent.click(dialog().getByRole('button', { name: 'Save model' }));

    expect(await screen.findByRole('listitem', { name: 'general' })).toBeInTheDocument();
    const models = lastUpdate()[1].models;
    expect(Object.keys(models).sort()).toEqual(['general', 'gpt-5']);
    expect(models.general).toEqual({ ...withSkills.spec.models.balanced, name: 'General' });
  });

  test('adds another model as a fallback in one click', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(card('openai').getByRole('button', { name: 'gpt-5' }));
    expect(dialog().getByRole('textbox', { name: /^Display name/ })).toHaveValue('');
    fireEvent.click(dialog().getByRole('button', { name: 'claude-sonnet-4-6 on anthropic' }));
    expect(dialog().queryByRole('button', { name: 'claude-sonnet-4-6 on anthropic' })).not.toBeInTheDocument();
    fireEvent.click(dialog().getByRole('button', { name: 'Save model' }));

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(lastUpdate()[1].models['gpt-5']).toEqual({
      name: 'gpt-5',
      primary: { provider: 'openai', model: 'gpt-5' },
      fallback: [{ provider: 'anthropic', model: 'claude-sonnet-4-6' }],
    });
  });

  test('marks a missing model ID only on save, and clears it once filled', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(card('openai').getByRole('button', { name: 'gpt-5' }));
    const id = dialog().getByRole('textbox', { name: /^Model ID/ });
    type(id, '');
    fireEvent.blur(id);
    expect(dialog().queryByText('Enter a model ID.')).not.toBeInTheDocument();
    fireEvent.click(dialog().getByRole('button', { name: 'Save model' }));

    expect(await dialog().findByText('Enter a model ID.')).toBeInTheDocument();
    expect(updateModelCatalog).not.toHaveBeenCalled();
    type(id, 'gpt-5-chat');
    expect(dialog().queryByText('Enter a model ID.')).not.toBeInTheDocument();
  });

  test('shows a model ID format error when the field is left', async () => {
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(card('openai').getByRole('button', { name: 'gpt-5' }));
    const id = dialog().getByRole('textbox', { name: /^Model ID/ });
    type(id, 'gpt 5');
    expect(dialog().queryByText(/Use up to 128 letters/)).not.toBeInTheDocument();
    fireEvent.blur(id);
    expect(dialog().getByText(/Use up to 128 letters/)).toBeInTheDocument();
  });

  test('removes the last model but disables removal of the last provider', async () => {
    const single = {
      ...stored,
      spec: {
        providers: { openai: stored.spec.providers.openai },
        models: { 'gpt-5': stored.spec.models['gpt-5'] },
      },
      secure: { openai: { name: 'inline-openai' } },
    };
    jest.mocked(getModelCatalog).mockResolvedValue(single);
    signInAs(OrgRole.Admin, ALL);
    renderPage();
    await screen.findByRole('listitem', { name: 'gpt-5' });

    fireEvent.click(card('openai').getByRole('button', { name: 'Remove gpt-5' }));
    fireEvent.click(dialog().getByRole('button', { name: 'Remove' }));
    expect(await card('openai').findByText('No models yet.')).toBeInTheDocument();
    expect(lastUpdate()).toEqual([single, { providers: single.spec.providers, models: {} }, {}]);
    expect(card('openai').queryByText('No key')).not.toBeInTheDocument();
    expect(card('openai').queryByText(SUCCESS_BADGES)).not.toBeInTheDocument();

    expect(
      card('openai').getByRole('button', { name: 'Delete the catalog to remove its last provider.' })
    ).toBeDisabled();
    expect(updateModelCatalog).toHaveBeenCalledTimes(1);
  });

  test('removes the provider of the last model without promoting its fallback', async () => {
    const lastModel = { ...stored, spec: { ...stored.spec, models: { balanced: stored.spec.models.balanced } } };
    jest.mocked(getModelCatalog).mockResolvedValue(lastModel);
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(card('anthropic').getByRole('button', { name: 'Remove anthropic' }));
    expect(dialog().getByText(/its models are removed: balanced/)).toBeInTheDocument();
    fireEvent.click(dialog().getByRole('button', { name: 'Remove' }));
    await waitFor(() => expect(screen.queryByRole('region', { name: 'anthropic' })).not.toBeInTheDocument());
    expect(lastUpdate()).toEqual([
      lastModel,
      { providers: { openai: stored.spec.providers.openai, spare: stored.spec.providers.spare }, models: {} },
      { anthropic: { remove: true } },
    ]);
    expect(card('openai').queryByText('No key')).not.toBeInTheDocument();
  });

  test('shows a refused change as returned in its drawer and keeps the input', async () => {
    const message = 'Operation cannot be fulfilled on modelcatalogs "team": the object has been modified';
    jest.mocked(updateModelCatalog).mockRejectedValue({ status: 409, data: { kind: 'Status', message } });
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(card('anthropic').getByRole('button', { name: 'balanced' }));
    type(dialog().getByRole('textbox', { name: /^Display name/ }), 'Stale');
    fireEvent.click(dialog().getByRole('button', { name: 'Save model' }));

    expect(await dialog().findByText(message)).toBeInTheDocument();
    expect(dialog().getByRole('textbox', { name: /^Display name/ })).toHaveValue('Stale');
  });

  test('shows a refused key as returned in its dialog without showing the key', async () => {
    jest.mocked(updateModelCatalog).mockRejectedValue({ status: 403, data: { message: 'forbidden: catalogwriter' } });
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(card('anthropic').getByRole('button', { name: 'Replace key' }));
    type(dialog().getByLabelText(/^New API key/), TYPED_KEY);
    fireEvent.click(dialog().getByRole('button', { name: 'Save key' }));

    expect(await dialog().findByText('forbidden: catalogwriter')).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent(TYPED_KEY);
  });

  test('shows a refused removal as returned on the page', async () => {
    jest.mocked(updateModelCatalog).mockRejectedValue({ status: 403, data: { message: 'forbidden: catalogwriter' } });
    signInAs(OrgRole.Admin, ALL);
    await openPage();

    fireEvent.click(card('openai').getByRole('button', { name: 'Remove gpt-5' }));
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));

    expect(await screen.findByText('forbidden: catalogwriter')).toBeInTheDocument();
    expect(screen.getByRole('listitem', { name: 'gpt-5' })).toBeInTheDocument();
  });

  test('shows a model read-only to readers', async () => {
    signInAs(OrgRole.Viewer, READ_ONLY);
    await openPage();

    fireEvent.click(card('anthropic').getByRole('button', { name: 'balanced' }));
    expect(dialog().getByRole('textbox', { name: /^Model ID/ })).toBeDisabled();
    expect(dialog().queryByRole('button', { name: 'Save model' })).not.toBeInTheDocument();
    fireEvent.click(dialog().getByRole('button', { name: 'Close' }));
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
