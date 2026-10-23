import React from 'react';
import { MemoryRouter } from 'react-router-dom';
import { AppRootProps, PluginType } from '@grafana/data';
import { render, screen, waitFor } from '@testing-library/react';
import App from './App';

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  config: { appUrl: 'https://grafana.example/', namespace: 'stacks-123' },
}));

describe('Components/App', () => {
  let props: AppRootProps;

  beforeEach(() => {
    jest.resetAllMocks();

    props = {
      basename: 'a/grafana-aigateway-app',
      meta: {
        id: 'grafana-aigateway-app',
        name: 'Grafana AI Gateway',
        type: PluginType.app,
        enabled: true,
        jsonData: {},
      },
      query: {},
      path: '',
      onNavChanged: jest.fn(),
    } as unknown as AppRootProps;
  });

  test.each(['/', '/overview', '/unknown'])('renders the overview at %s', async (path) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <App {...props} />
      </MemoryRouter>
    );

    await waitFor(() => expect(screen.getByText('Quick start')).toBeInTheDocument(), { timeout: 2000 });
  });
});
