import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import Overview, { gatewayEndpoint } from './Overview';
import { LINKS } from './overview/content';

const ENDPOINT = 'https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123';

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  config: { appUrl: 'https://grafana.example/', appSubUrl: '/grafana', namespace: 'stacks-123' },
}));

describe('pages/Overview', () => {
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

  test('links to service account creation in this Grafana', () => {
    render(<Overview />);

    expect(screen.getByRole('link', { name: /Create service account/ })).toHaveAttribute(
      'href',
      '/grafana/org/serviceaccounts/new'
    );
  });

  test('switches the quick start between TypeScript and Go with the stack endpoint and provider key', () => {
    render(<Overview />);

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
    render(<Overview />);

    expect(screen.getByRole('link', { name: /Your application/ })).toHaveAttribute('href', LINKS.byok);
    expect(screen.getByRole('link', { name: /A Grafana service/ })).toHaveAttribute('href', LINKS.grafanaService);
    expect(screen.getByRole('link', { name: /An App Platform app/ })).toHaveAttribute('href', LINKS.appPlatform);
  });

  test('keeps self-hosted guidance collapsed until opened', () => {
    render(<Overview />);

    expect(screen.queryByRole('link', { name: /Configure models/ })).not.toBeInTheDocument();

    fireEvent.click(screen.getByText('Running Grafana yourself?'));

    expect(screen.getByRole('link', { name: /Configure models/ })).toHaveAttribute('href', LINKS.configureModels);
  });
});
