import React from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { OrgRole } from '@grafana/data';
import { PluginPageProps } from '@grafana/runtime';
import { render, screen } from '@testing-library/react';
import CatalogList from './CatalogList';
import { listModelCatalogs } from '../api/modelCatalogs';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { CatalogActions } from '../permissions';
import { signInAs } from '../testUtils';

jest.mock('../api/modelCatalogs', () => ({
  ...jest.requireActual('../api/modelCatalogs'),
  listModelCatalogs: jest.fn(),
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
      </Routes>
    </MemoryRouter>
  );
}

describe('pages/CatalogList', () => {
  beforeEach(() => {
    jest
      .mocked(listModelCatalogs)
      .mockReset()
      .mockResolvedValue([catalog('team')]);
  });

  test('shows each catalog with its URL, counts and provider types, never secure names', async () => {
    signInAs(OrgRole.Viewer, READ_ONLY);
    renderList();

    expect(await screen.findByRole('link', { name: 'team' })).toHaveAttribute('href', `${APP}/catalogs/edit/team`);
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
  ])('offers create to %s only with the action, and delete to nobody', async (_, role, actions, writes) => {
    signInAs(role, actions);
    renderList();

    await screen.findByRole('link', { name: 'team' });
    expect(screen.queryByRole('link', { name: 'New catalog' }) !== null).toBe(writes);
    expect(screen.queryByRole('button', { name: /Delete/ })).not.toBeInTheDocument();
  });

  test('links New catalog to the create page', async () => {
    signInAs(OrgRole.Admin, ALL);
    renderList();

    expect(await screen.findByRole('link', { name: 'New catalog' })).toHaveAttribute('href', `${APP}/catalogs/new`);
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
      expect(screen.queryAllByRole('link', { name: 'New catalog' })).toHaveLength(offered ? 1 : 0);
    },
    EMPTY_STATE_TIMEOUT_MS
  );

  test('shows the message of a failed list as returned', async () => {
    jest.mocked(listModelCatalogs).mockRejectedValue({ status: 500, data: { message: 'storage unavailable' } });
    signInAs(OrgRole.Viewer, READ_ONLY);
    renderList();

    expect(await screen.findByText('storage unavailable')).toBeInTheDocument();
  });
});
