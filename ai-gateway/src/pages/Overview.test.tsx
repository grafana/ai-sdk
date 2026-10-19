import React from 'react';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { testIds } from '../components/testIds';
import Overview, { gatewayEndpoint } from './Overview';

const ENDPOINT = 'https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123';

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  config: { appUrl: 'https://grafana.example/', namespace: 'stacks-123' },
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

    expect(within(screen.getByTestId(testIds.overview.endpoint)).getByText(ENDPOINT)).toBeInTheDocument();
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
});
