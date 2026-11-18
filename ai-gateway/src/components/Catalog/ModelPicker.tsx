import React, { useRef, useState } from 'react';
import { Button, Combobox, Stack } from '@grafana/ui';
import { ProviderModelListing } from '../../api/modelCatalogs';

type State =
  { kind: 'closed' } | { kind: 'loading' } | { kind: 'ready'; listing: ProviderModelListing } | { kind: 'failed' };

const MESSAGES: Record<ProviderModelListing['status'], string> = {
  ok: 'The provider returned no models.',
  partial: 'The provider listing is incomplete. You can still enter any model ID.',
  invalidKey: 'The provider rejected the saved key.',
  unreachable: 'The provider could not be reached.',
  refused: 'The provider did not return a usable model listing.',
  notChecked: 'This provider listing is unavailable.',
};

interface Props {
  listModels: () => Promise<ProviderModelListing>;
  onSelect: (id: string) => void;
}

/** ModelPicker looks up one saved account's suggestions without submitting the model form. */
export function ModelPicker({ listModels, onSelect }: Props) {
  const [state, setState] = useState<State>({ kind: 'closed' });
  const generation = useRef(0);

  const close = () => {
    generation.current++;
    setState({ kind: 'closed' });
  };

  const browse = async () => {
    const current = ++generation.current;
    setState({ kind: 'loading' });
    try {
      const listing = await listModels();
      if (current === generation.current) {
        setState({ kind: 'ready', listing });
      }
    } catch {
      if (current === generation.current) {
        setState({ kind: 'failed' });
      }
    }
  };

  if (state.kind === 'closed') {
    return (
      <Button type="button" variant="secondary" fill="text" onClick={browse}>
        Browse models
      </Button>
    );
  }

  const listing = state.kind === 'ready' ? state.listing : undefined;
  const models = listing?.modelIds ?? [];
  const message =
    state.kind === 'loading'
      ? 'Loading provider models…'
      : state.kind === 'failed'
        ? 'Could not list models. Try again or enter a model ID.'
        : listing && (listing.status !== 'ok' || models.length === 0)
          ? MESSAGES[listing.status]
          : undefined;

  return (
    <Stack direction="column" gap={1}>
      <div role="status">{message}</div>
      {models.length > 0 && (
        <Combobox
          aria-label="Search provider models"
          placeholder="Search provider models"
          noOptionsMessage="No matching models. You can enter a model ID."
          options={models.map((value) => ({ value }))}
          value={null}
          onChange={({ value }) => {
            onSelect(value);
            close();
          }}
        />
      )}
      <Stack gap={1}>
        {(state.kind === 'failed' || (listing && listing.status !== 'ok')) && (
          <Button type="button" variant="secondary" onClick={browse}>
            Retry listing
          </Button>
        )}
        <Button type="button" variant="secondary" fill="text" onClick={close}>
          Close model picker
        </Button>
      </Stack>
    </Stack>
  );
}
