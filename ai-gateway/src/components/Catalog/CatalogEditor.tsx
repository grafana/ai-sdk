import React, { useState } from 'react';
import { Alert, Button, ConfirmModal, Stack, Text } from '@grafana/ui';
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
  stored: boolean;
  health?: CatalogHealth;
  readOnly: boolean;
  listingRevision?: string;
  listModels?: (provider: string) => Promise<ProviderModelListing>;
  onChange: (change: CatalogChange) => Promise<void>;
}

/**
 * CatalogEditor shows a catalog's providers, each with the models it serves,
 * and applies each change a writer confirms through onChange: a write for a
 * stored catalog, or the draft of a new one. A stored catalog keeps at least
 * one model, and a provider can't be removed while other providers' models
 * fall back to it.
 */
export function CatalogEditor({
  spec,
  keys,
  stored,
  health,
  readOnly,
  listingRevision,
  listModels,
  onChange,
}: Props) {
  const [dialog, setDialog] = useState<Dialog>();
  const [error, setError] = useState<string>();
  const providers = Object.entries(spec.providers);
  const modelCount = Object.keys(spec.models).length;
  const close = () => setDialog(undefined);

  const save = async (change: CatalogChange) => {
    await onChange(change);
    setError(undefined);
    close();
  };

  const confirm = async (change?: CatalogChange) => {
    close();
    if (change === undefined) {
      return;
    }
    try {
      await onChange(change);
      setError(undefined);
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const removeBlocked = (provider: string): string | undefined => {
    const users = fallbackUsers(spec, provider);
    if (users.length > 0) {
      return `${provider} can't be removed while ${users.join(', ')} ${users.length === 1 ? 'falls' : 'fall'} back to it`;
    }
    const own = modelsOf(spec, provider).length;
    return stored && own > 0 && own === modelCount
      ? `${provider} serves every model, and a catalog needs at least one; delete the catalog instead`
      : undefined;
  };

  const removalText = (provider: string) => {
    const own = modelsOf(spec, provider).map(([id]) => id);
    const key = stored ? 'Its saved API key is deleted' : 'Its API key is discarded';
    return `Remove ${provider}? ${key}${own.length > 0 ? `, and its models are removed: ${own.join(', ')}` : ''}.`;
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
      {providers.length === 0 && (
        <Text color="secondary">Connect a provider with its API key, then add the models applications can use.</Text>
      )}
      {providers.map(([name, provider]) => (
        <ProviderCard
          key={name}
          name={name}
          provider={provider}
          models={modelsOf(spec, name)}
          keyState={!keys.includes(name) ? 'missing' : stored ? undefined : 'entered'}
          health={health}
          readOnly={readOnly}
          listingRevision={listingRevision}
          listModels={stored && keys.includes(name) && listModels ? () => listModels(name) : undefined}
          removeBlocked={removeBlocked(name)}
          lastModel={stored && modelCount === 1}
          onOpenModel={(id) => setDialog({ kind: 'model', id })}
          onRemoveModel={(id) => setDialog({ kind: 'removeModel', id })}
          onAddModel={(providerModel) =>
            onChange({ spec: withModel(spec, providerModel, quickModel(name, providerModel)), secure: {} })
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
