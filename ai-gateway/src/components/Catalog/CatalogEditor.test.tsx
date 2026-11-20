import React from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { ProviderModelListing } from '../../api/modelCatalogs';
import { CatalogEditor } from './CatalogEditor';
import { ModelPicker } from './ModelPicker';
import { ProviderCard } from './ProviderCard';

jest.mock('./ProviderCard', () => ({ ProviderCard: jest.fn() }));

it('shares one pending listing across two fields for the same saved provider', async () => {
  jest.mocked(ProviderCard).mockImplementation(({ name, listModels }) =>
    listModels ? (
      <>
        <ModelPicker
          label={`${name} primary`}
          value=""
          onChange={jest.fn()}
          onBlur={jest.fn()}
          listModels={listModels}
        />
        <ModelPicker
          label={`${name} fallback`}
          value=""
          onChange={jest.fn()}
          onBlur={jest.fn()}
          listModels={listModels}
        />
      </>
    ) : (
      <></>
    )
  );

  let resolve!: (listing: ProviderModelListing) => void;
  const pending = new Promise<ProviderModelListing>((finish) => {
    resolve = finish;
  });
  const listModels = jest.fn(() => pending);
  render(
    <CatalogEditor
      keys={['anthropic']}
      spec={{ providers: { anthropic: { type: 'anthropic' } }, models: {} }}
      readOnly={false}
      listModels={listModels}
      onChange={jest.fn()}
    />
  );

  fireEvent.click(screen.getByRole('combobox', { name: 'anthropic primary' }));
  fireEvent.click(screen.getByRole('combobox', { name: 'anthropic fallback' }));
  expect(listModels).toHaveBeenCalledTimes(1);
  expect(listModels).toHaveBeenCalledWith('anthropic');
  await act(() => resolve({ status: 'ok', modelIds: ['shared-model'] }));
  expect(screen.getAllByRole('option', { name: 'shared-model' })).toHaveLength(2);
});
