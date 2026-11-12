import React from 'react';
import { OrgRole } from '@grafana/data';
import { fireEvent, render, screen } from '@testing-library/react';
import Overview, { catalogBaseURL, gatewayEndpoint } from './Overview';
import { listModelCatalogs } from '../api/modelCatalogs';
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
  config: { appUrl: 'https://grafana.example/', namespace: 'stacks-123' },
}));

function catalog(name: string): ModelCatalog {
  return {
    apiVersion: 'aigateway.ext.grafana.app/v1alpha1',
    kind: 'ModelCatalog',
    metadata: { name, namespace: 'stacks-123' },
    spec: { providers: {}, models: {} },
  };
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
    render(<Overview />);

    expect(screen.getByText(ENDPOINT)).toBeInTheDocument();
  });

  test('switches the quick start between TypeScript and Go with the stack endpoint', () => {
    render(<Overview />);

    expect(screen.getByText('npm install ai @ai-sdk/gateway')).toBeInTheDocument();
    expect(screen.getByText(/createGateway\(\{/)).toHaveTextContent(`baseURL: '${ENDPOINT}'`);

    fireEvent.click(screen.getByText('Go'));

    expect(screen.getByText(/go get github\.com\/grafana\/ai-sdk/)).toBeInTheDocument();
    expect(screen.getByText(/NewWithServiceAccountToken/)).toHaveTextContent(`BaseURL: "${ENDPOINT}"`);
  });

  test('links to the Gateway documentation', () => {
    render(<Overview />);

    expect(screen.getByRole('link', { name: /Authenticate callers/ })).toHaveAttribute(
      'href',
      'https://github.com/grafana/ai-sdk/blob/main/docs/guides/gateway-authentication.md'
    );
  });

  test('derives a catalog base URL from the endpoint', () => {
    expect(catalogBaseURL(ENDPOINT, 'team')).toBe(`${ENDPOINT}/modelcatalogs/team`);
  });

  test('shows each catalog base URL to catalog readers', async () => {
    jest.mocked(listModelCatalogs).mockResolvedValue([catalog('team'), catalog('ops')]);
    signInAs(OrgRole.Viewer, [CatalogActions.Read]);
    render(<Overview />);

    expect(await screen.findByText(`${ENDPOINT}/modelcatalogs/team`)).toBeInTheDocument();
    expect(screen.getByText(`${ENDPOINT}/modelcatalogs/ops`)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Manage model catalogs/ })).toHaveAttribute(
      'href',
      '/a/grafana-aigateway-app/catalogs'
    );
  });

  test('says when the stack has no catalogs', async () => {
    signInAs(OrgRole.Viewer, [CatalogActions.Read]);
    render(<Overview />);

    expect(await screen.findByText('This stack has no model catalogs yet.')).toBeInTheDocument();
  });

  test('requests no catalogs without the read action', () => {
    signInAs(OrgRole.Admin, []);
    render(<Overview />);

    expect(screen.queryByText('Model catalogs')).not.toBeInTheDocument();
    expect(listModelCatalogs).not.toHaveBeenCalled();
    expect(screen.getByText(ENDPOINT)).toBeInTheDocument();
  });

  test('shows a failed catalog list inline and keeps the rest of the page', async () => {
    jest.mocked(listModelCatalogs).mockRejectedValue({ status: 503, data: { message: 'kind not served' } });
    signInAs(OrgRole.Viewer, [CatalogActions.Read]);
    render(<Overview />);

    expect(await screen.findByText('kind not served')).toBeInTheDocument();
    expect(screen.getByText(ENDPOINT)).toBeInTheDocument();
    expect(screen.getByText('Quick start')).toBeInTheDocument();
  });
});
