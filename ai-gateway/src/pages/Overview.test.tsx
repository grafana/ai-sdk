import React from 'react';
import { MemoryRouter } from 'react-router-dom';
import { OrgRole } from '@grafana/data';
import { fireEvent, render, screen } from '@testing-library/react';
import Overview from './Overview';
import { LINKS } from './overview/content';
import { listModelCatalogs } from '../api/modelCatalogs';
import { catalogBaseURL, gatewayEndpoint } from '../constants';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { CatalogActions } from '../permissions';
import { signInAs } from '../testUtils';

const ENDPOINT = 'https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123';

jest.mock('../api/modelCatalogs', () => ({
  ...jest.requireActual('../api/modelCatalogs'),
  listModelCatalogs: jest.fn(),
}));

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  config: { appUrl: 'https://grafana.example/', appSubUrl: '/grafana', namespace: 'stacks-123' },
}));

function catalog(name: string): ModelCatalog {
  return {
    apiVersion: 'aigateway.ext.grafana.app/v1alpha1',
    kind: 'ModelCatalog',
    metadata: { name, namespace: 'stacks-123' },
    spec: { providers: {}, models: {} },
  };
}

function renderOverview() {
  return render(
    <MemoryRouter>
      <Overview />
    </MemoryRouter>
  );
}

describe('pages/Overview', () => {
  beforeEach(() => {
    jest.mocked(listModelCatalogs).mockReset().mockResolvedValue([]);
    signInAs(OrgRole.Viewer, []);
  });

  test.each([
    ['https://grafana.example/', 'stacks-123', ENDPOINT],
    [
      'https://grafana.example/grafana//',
      'default',
      'https://grafana.example/grafana/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/default',
    ],
    [
      'https://grafana.example',
      'org 2',
      'https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/org%202',
    ],
  ])('derives the endpoint from %s and %s', (appUrl, namespace, expected) => {
    expect(gatewayEndpoint(appUrl, namespace)).toBe(expected);
  });

  test('shows this stack endpoint', () => {
    renderOverview();

    expect(screen.getByText(ENDPOINT)).toBeInTheDocument();
  });

  test('links to service account creation in this Grafana', () => {
    renderOverview();

    expect(screen.getByRole('link', { name: /Create service account/ })).toHaveAttribute(
      'href',
      '/grafana/org/serviceaccounts/new'
    );
  });

  test('switches the quick start between TypeScript and Go with the stack endpoint and provider key', () => {
    renderOverview();

    expect(screen.getByText('npm install ai @ai-sdk/gateway')).toBeInTheDocument();
    const typeScript = screen.getByText(/createGateway\(\{/);
    expect(typeScript).toHaveTextContent(`baseURL: '${ENDPOINT}'`);
    expect(typeScript).toHaveTextContent('byok: { anthropic: [{ apiKey: process.env.ANTHROPIC_API_KEY }] }');

    fireEvent.click(screen.getByText('Go'));

    expect(screen.getByText(/go get github\.com\/grafana\/ai-sdk/)).toBeInTheDocument();
    const goCode = screen.getByText(/NewWithServiceAccountToken/);
    expect(goCode).toHaveTextContent(`BaseURL: "${ENDPOINT}"`);
    expect(goCode).toHaveTextContent('"anthropic": {{APIKey: os.Getenv("ANTHROPIC_API_KEY")}}');
  });

  test('links each kind of caller to its authentication guide', () => {
    renderOverview();

    expect(screen.getByRole('link', { name: /Your application/ })).toHaveAttribute('href', LINKS.byok);
    expect(screen.getByRole('link', { name: /A Grafana service/ })).toHaveAttribute('href', LINKS.grafanaService);
    expect(screen.getByRole('link', { name: /An App Platform app/ })).toHaveAttribute('href', LINKS.appPlatform);
  });

  test('keeps self-hosted guidance collapsed until opened', () => {
    renderOverview();

    expect(screen.queryByRole('link', { name: /Configure models/ })).not.toBeInTheDocument();

    fireEvent.click(screen.getByText('Running Grafana yourself?'));

    expect(screen.getByRole('link', { name: /Configure models/ })).toHaveAttribute('href', LINKS.configureModels);
  });

  test('derives a catalog base URL from the endpoint', () => {
    expect(catalogBaseURL(ENDPOINT, 'team')).toBe(`${ENDPOINT}/modelcatalogs/team`);
  });

  test('shows each catalog base URL to catalog readers', async () => {
    jest.mocked(listModelCatalogs).mockResolvedValue([catalog('team'), catalog('ops')]);
    signInAs(OrgRole.Viewer, [CatalogActions.Read]);
    renderOverview();

    expect(await screen.findByText(`${ENDPOINT}/modelcatalogs/team`)).toBeInTheDocument();
    expect(screen.getByText(`${ENDPOINT}/modelcatalogs/ops`)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'team' })).toHaveAttribute(
      'href',
      '/a/grafana-aigateway-app/catalogs/team'
    );
    expect(screen.getByRole('link', { name: /Manage model catalogs/ })).toHaveAttribute(
      'href',
      '/a/grafana-aigateway-app/catalogs'
    );
  });

  test('says when the stack has no catalogs', async () => {
    signInAs(OrgRole.Viewer, [CatalogActions.Read]);
    renderOverview();

    expect(await screen.findByText('This stack has no model catalogs yet.')).toBeInTheDocument();
  });

  test('requests no catalogs without the read action', () => {
    signInAs(OrgRole.Admin, []);
    renderOverview();

    expect(screen.queryByText('Model catalogs')).not.toBeInTheDocument();
    expect(listModelCatalogs).not.toHaveBeenCalled();
    expect(screen.getByText(ENDPOINT)).toBeInTheDocument();
  });

  test('shows a failed catalog list inline and keeps the rest of the page', async () => {
    jest.mocked(listModelCatalogs).mockRejectedValue({ status: 503, data: { message: 'kind not served' } });
    signInAs(OrgRole.Viewer, [CatalogActions.Read]);
    renderOverview();

    expect(await screen.findByText('kind not served')).toBeInTheDocument();
    expect(screen.getByText(ENDPOINT)).toBeInTheDocument();
    expect(screen.getByText('Get started in Grafana Cloud')).toBeInTheDocument();
  });
});
