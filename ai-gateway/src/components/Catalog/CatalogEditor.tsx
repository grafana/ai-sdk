import React, { useCallback, useRef, useState } from 'react';
import { Alert, Button, ConfirmModal, Stack } from '@grafana/ui';
import { CatalogHealth, errorMessage, ProviderModelListing } from '../../api/modelCatalogs';
import { ConnectProviderModal } from './ConnectProviderModal';
import { ModelDrawer } from './ModelDrawer';
import { ProviderCard } from './ProviderCard';
import { ReplaceKeyModal } from './ReplaceKeyModal';
import {
  CatalogChange,
  fallbackUsers,
  keyRemoval,
  modelsOf,
  quickAddError,
  quickModel,
  withModel,
  withoutModel,
  withoutProvider,
} from './catalogSpec';
import { Spec } from '../../generated/modelcatalog/v1alpha1/types.spec.gen';

type Dialog =
  | { kind: 'connect' }
  | { kind: 'replaceKey'; provider: string }
  | { kind: 'model'; id: string }
  | { kind: 'removeProvider'; provider: string }
  | { kind: 'removeModel'; id: string };

interface Props {
  spec: Spec;
  keys: string[];
  health?: CatalogHealth;
  readOnly: boolean;
  listingRevision?: string;
  listModels?: (provider: string) => Promise<ProviderModelListing>;
  onChange: (change: CatalogChange) => Promise<void>;
}

/**
 * CatalogEditor shows a catalog's providers, each with the models it serves,
 * and applies each change a writer confirms through onChange. A provider
 * can't be removed if it is the last provider or another provider's model falls back to it.
 */
export function CatalogEditor({ spec, keys, health, readOnly, listingRevision, listModels, onChange }: Props) {
  const [dialog, setDialog] = useState<Dialog>();
  const [error, setError] = useState<string>();
  const pendingListings = useRef(new Map<string, Promise<ProviderModelListing>>());
  const listSavedModels = useCallback((provider: string, load: NonNullable<Props['listModels']>) => {
    const requests = pendingListings.current;
    const pending = requests.get(provider);
    if (pending) {
      return pending;
    }
    const request = load(provider).finally(() => {
      if (requests.get(provider) === request) {
        requests.delete(provider);
      }
    });
    requests.set(provider, request);
    return request;
  }, []);
  const providers = Object.entries(spec.providers);
  const close = () => setDialog(undefined);

  const applyChange = async (change: CatalogChange) => {
    await onChange(change);
    pendingListings.current.clear();
  };

  const save = async (change: CatalogChange) => {
    await applyChange(change);
    setError(undefined);
    close();
  };

  const confirm = async (change?: CatalogChange) => {
    close();
    if (change === undefined) {
      return;
    }
    try {
      await applyChange(change);
      setError(undefined);
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const removeBlocked = (provider: string): string | undefined => {
    if (providers.length === 1) {
      return 'Delete the catalog to remove its last provider.';
    }
    const users = fallbackUsers(spec, provider);
    return users.length > 0
      ? `${provider} can't be removed while ${users.join(', ')} ${users.length === 1 ? 'falls' : 'fall'} back to it`
      : undefined;
  };

  const removalText = (provider: string) => {
    const own = modelsOf(spec, provider).map(([id]) => id);
    return `Remove ${provider}? Its saved API key is deleted${own.length > 0 ? `, and its models are removed: ${own.join(', ')}` : ''}.`;
  };

  const removal = (() => {
    switch (dialog?.kind) {
      case 'removeProvider':
        return {
          title: 'Remove provider',
          body: removalText(dialog.provider),
          change: { spec: withoutProvider(spec, dialog.provider), secure: keyRemoval(dialog.provider, keys) },
        };
      case 'removeModel':
        return {
          title: 'Remove model',
          body: `Remove ${dialog.id}? Applications that request it get an error.`,
          change: { spec: withoutModel(spec, dialog.id), secure: {} },
        };
      default:
        return undefined;
    }
  })();

  return (
    <Stack direction="column" gap={2}>
      {error && (
        <Alert severity="error" title="The change wasn't saved" onRemove={() => setError(undefined)}>
          {error}
        </Alert>
      )}
      {providers.map(([name, provider]) => (
        <ProviderCard
          key={name}
          name={name}
          provider={provider}
          models={modelsOf(spec, name)}
          hasKey={keys.includes(name)}
          health={health}
          readOnly={readOnly}
          listingRevision={listingRevision}
          listModels={keys.includes(name) && listModels ? () => listSavedModels(name, listModels) : undefined}
          removeBlocked={removeBlocked(name)}
          onOpenModel={(id) => setDialog({ kind: 'model', id })}
          onRemoveModel={(id) => setDialog({ kind: 'removeModel', id })}
          onAddModel={(providerModel) =>
            applyChange({ spec: withModel(spec, providerModel, quickModel(name, providerModel)), secure: {} })
          }
          addError={(providerModel) => quickAddError(spec, providerModel)}
          onReplaceKey={() => setDialog({ kind: 'replaceKey', provider: name })}
          onRemove={() => setDialog({ kind: 'removeProvider', provider: name })}
        />
      ))}
      {!readOnly && (
        <div>
          <Button variant="secondary" icon="plus" onClick={() => setDialog({ kind: 'connect' })}>
            Connect a provider
          </Button>
        </div>
      )}

      {dialog?.kind === 'connect' && <ConnectProviderModal spec={spec} onSave={save} onDismiss={close} />}
      {dialog?.kind === 'replaceKey' && (
        <ReplaceKeyModal spec={spec} provider={dialog.provider} onSave={save} onDismiss={close} />
      )}
      {dialog?.kind === 'model' && (
        <ModelDrawer
          spec={spec}
          id={dialog.id}
          fallbackHealth={health?.models[dialog.id]?.fallback}
          readOnly={readOnly}
          onSave={save}
          onClose={close}
        />
      )}
      <ConfirmModal
        isOpen={removal !== undefined}
        title={removal?.title ?? ''}
        body={removal?.body ?? ''}
        confirmText="Remove"
        onConfirm={() => confirm(removal?.change)}
        onDismiss={close}
      />
    </Stack>
  );
}
