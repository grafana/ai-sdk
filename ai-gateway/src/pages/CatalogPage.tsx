import React, { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { config, PluginPage } from '@grafana/runtime';
import {
  Alert,
  Badge,
  Button,
  ConfirmModal,
  IconButton,
  LoadingPlaceholder,
  Stack,
  Text,
  useStyles2,
} from '@grafana/ui';
import {
  deleteModelCatalog,
  errorMessage,
  getModelCatalog,
  SecureChanges,
  updateModelCatalog,
} from '../api/modelCatalogs';
import { CatalogURL } from '../components/Catalog/CatalogURL';
import { ModelModal } from '../components/Catalog/ModelModal';
import { Panel, PanelRow } from '../components/Catalog/Panel';
import { ProviderModal } from '../components/Catalog/ProviderModal';
import {
  keyRemoval,
  modelsUsing,
  providerTypeLabel,
  withoutModel,
  withoutProvider,
} from '../components/Catalog/catalogSpec';
import { NoCatalogAccess } from '../components/NoCatalogAccess/NoCatalogAccess';
import { catalogBaseURL, gatewayEndpoint, prefixRoute, ROUTES } from '../constants';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { Model, Spec, Target } from '../generated/modelcatalog/v1alpha1/types.spec.gen';
import { catalogPermissions } from '../permissions';

type Dialog =
  | { kind: 'provider'; name?: string }
  | { kind: 'model'; id?: string }
  | { kind: 'removeProvider'; name: string }
  | { kind: 'removeModel'; id: string }
  | { kind: 'deleteCatalog' };

function route(target: Target): string {
  return `${target.provider} › ${target.model}`;
}

function routeSummary(model: Model): string {
  const fallback = (model.fallback ?? []).map(route);
  return [`Calls ${route(model.primary)}`, ...fallback.map((target) => `then ${target}`)].join(', ');
}

interface Props {
  name: string;
}

/**
 * CatalogView shows a catalog's URL, providers and models. A writer adds,
 * edits and removes them one at a time; each change is saved at once, as an
 * update of the stored object.
 */
function CatalogView({ name }: Props) {
  const s = useStyles2(getStyles);
  const permissions = catalogPermissions();
  const navigate = useNavigate();
  const [stored, setStored] = useState<ModelCatalog>();
  const [loadError, setLoadError] = useState<string>();
  const [actionError, setActionError] = useState<string>();
  const [dialog, setDialog] = useState<Dialog>();

  useEffect(() => {
    if (!permissions.read) {
      return;
    }
    let current = true;
    getModelCatalog(name).then(
      (catalog) => current && setStored(catalog),
      (err) => current && setLoadError(errorMessage(err))
    );
    return () => {
      current = false;
    };
  }, [name, permissions.read]);

  const storedKeys = Object.keys(stored?.secure ?? {});
  const endpoint = gatewayEndpoint(config.appUrl, config.namespace);

  const save = async (catalog: ModelCatalog, spec: Spec, secure: SecureChanges = {}) => {
    setStored(await updateModelCatalog(catalog, spec, secure));
    setActionError(undefined);
    setDialog(undefined);
  };

  const confirm = async (action: () => Promise<void>) => {
    setDialog(undefined);
    try {
      await action();
    } catch (err) {
      setActionError(errorMessage(err));
    }
  };

  const content = (catalog: ModelCatalog) => {
    const { spec } = catalog;
    const providers = Object.entries(spec.providers);
    const models = Object.entries(spec.models);
    const editable = permissions.update;

    return (
      <Stack direction="column" gap={3}>
        <Panel
          title="Providers"
          description="The provider accounts this catalog calls, each with its own API key."
          action={
            editable && (
              <Button variant="secondary" icon="plus" onClick={() => setDialog({ kind: 'provider' })}>
                Add provider
              </Button>
            )
          }
        >
          {providers.map(([provider, { type }]) => {
            const users = modelsUsing(spec, provider);
            return (
              <PanelRow
                key={provider}
                label={provider}
                actions={
                  editable && (
                    <>
                      <IconButton
                        name="pen"
                        tooltip={`Edit ${provider}`}
                        onClick={() => setDialog({ kind: 'provider', name: provider })}
                      />
                      <IconButton
                        name="trash-alt"
                        tooltip={
                          users.length > 0 ? `${provider} can't be removed while models use it` : `Remove ${provider}`
                        }
                        disabled={users.length > 0}
                        onClick={() => setDialog({ kind: 'removeProvider', name: provider })}
                      />
                    </>
                  )
                }
              >
                <Stack direction="column" gap={0.5}>
                  <Stack alignItems="center" gap={1} wrap="wrap">
                    <Text weight="medium">{provider}</Text>
                    <Badge text={providerTypeLabel(type)} color="blue" />
                    {storedKeys.includes(provider) ? (
                      <Badge text="Key saved" icon="lock" color="green" />
                    ) : (
                      <Badge text="No key" icon="exclamation-triangle" color="orange" />
                    )}
                  </Stack>
                  <Text color="secondary" variant="bodySmall">
                    {users.length > 0 ? `Used by ${users.join(', ')}` : 'No model uses this provider'}
                  </Text>
                </Stack>
              </PanelRow>
            );
          })}
        </Panel>

        <Panel
          title="Models"
          description="The models applications can request from this catalog, and the providers each one calls."
          action={
            editable && (
              <Button variant="secondary" icon="plus" onClick={() => setDialog({ kind: 'model' })}>
                Add model
              </Button>
            )
          }
        >
          {models.map(([id, model]) => (
            <PanelRow
              key={id}
              label={id}
              actions={
                editable && (
                  <>
                    <IconButton name="pen" tooltip={`Edit ${id}`} onClick={() => setDialog({ kind: 'model', id })} />
                    <IconButton
                      name="trash-alt"
                      tooltip={models.length > 1 ? `Remove ${id}` : 'A catalog needs at least one model'}
                      disabled={models.length === 1}
                      onClick={() => setDialog({ kind: 'removeModel', id })}
                    />
                  </>
                )
              }
            >
              <Stack direction="column" gap={0.5}>
                <Stack alignItems="baseline" gap={1} wrap="wrap">
                  <Text weight="medium">{id}</Text>
                  <Text color="secondary">{model.name}</Text>
                </Stack>
                {model.description && (
                  <Text color="secondary" variant="bodySmall">
                    {model.description}
                  </Text>
                )}
                <Text color="secondary" variant="bodySmall">
                  {routeSummary(model)}
                </Text>
                {model.aliases && model.aliases.length > 0 && (
                  <Text color="secondary" variant="bodySmall">
                    Aliases: {model.aliases.join(', ')}
                  </Text>
                )}
              </Stack>
            </PanelRow>
          ))}
        </Panel>

        {dialog?.kind === 'provider' && (
          <ProviderModal
            spec={spec}
            storedKeys={storedKeys}
            name={dialog.name}
            onSave={(next, secure) => save(catalog, next, secure)}
            onDismiss={() => setDialog(undefined)}
          />
        )}
        {dialog?.kind === 'model' && (
          <ModelModal
            spec={spec}
            id={dialog.id}
            onSave={(next) => save(catalog, next)}
            onDismiss={() => setDialog(undefined)}
          />
        )}
        <ConfirmModal
          isOpen={dialog?.kind === 'removeProvider'}
          title="Remove provider"
          body={`Remove the provider "${dialog?.kind === 'removeProvider' ? dialog.name : ''}"? Its saved API key is deleted.`}
          confirmText="Remove"
          onConfirm={() =>
            confirm(async () => {
              if (dialog?.kind === 'removeProvider') {
                await save(catalog, withoutProvider(spec, dialog.name), keyRemoval(dialog.name, storedKeys));
              }
            })
          }
          onDismiss={() => setDialog(undefined)}
        />
        <ConfirmModal
          isOpen={dialog?.kind === 'removeModel'}
          title="Remove model"
          body={`Remove the model "${dialog?.kind === 'removeModel' ? dialog.id : ''}"? Applications that request it get an error.`}
          confirmText="Remove"
          onConfirm={() =>
            confirm(async () => {
              if (dialog?.kind === 'removeModel') {
                await save(catalog, withoutModel(spec, dialog.id));
              }
            })
          }
          onDismiss={() => setDialog(undefined)}
        />
      </Stack>
    );
  };

  const body = () => {
    if (!permissions.read) {
      return <NoCatalogAccess />;
    }
    if (!stored) {
      return loadError ? null : <LoadingPlaceholder text="Loading catalog..." />;
    }
    return content(stored);
  };

  return (
    <PluginPage
      pageNav={{ text: name }}
      subTitle={stored && <CatalogURL name={name} url={catalogBaseURL(endpoint, name)} />}
      actions={
        stored &&
        permissions.delete && (
          <Button
            variant="destructive"
            fill="outline"
            icon="trash-alt"
            onClick={() => setDialog({ kind: 'deleteCatalog' })}
          >
            Delete catalog
          </Button>
        )
      }
    >
      <div className={s.page}>
        <Stack direction="column" gap={2}>
          {loadError && (
            <Alert severity="error" title="The catalog couldn't be loaded">
              {loadError}
            </Alert>
          )}
          {actionError && (
            <Alert severity="error" title="The change wasn't saved" onRemove={() => setActionError(undefined)}>
              {actionError}
            </Alert>
          )}
          {stored && !permissions.update && (
            <Alert severity="info" title="Read-only">
              You can view this catalog. Changing it needs the Model catalog writer role, which Admins have by default.
            </Alert>
          )}
          {body()}
        </Stack>
      </div>
      <ConfirmModal
        isOpen={dialog?.kind === 'deleteCatalog'}
        title="Delete model catalog"
        body={`Delete the catalog "${name}"? Applications that call its URL stop working, and its saved keys are deleted.`}
        confirmText="Delete"
        onConfirm={() =>
          confirm(async () => {
            await deleteModelCatalog(name);
            navigate(prefixRoute(ROUTES.Catalogs));
          })
        }
        onDismiss={() => setDialog(undefined)}
      />
    </PluginPage>
  );
}

/** CatalogPage is the page of the route's catalog. */
function CatalogPage() {
  const { name = '' } = useParams();
  return <CatalogView key={name} name={name} />;
}

export default CatalogPage;

const getStyles = (theme: GrafanaTheme2) => ({
  page: css({
    maxWidth: theme.breakpoints.values.lg,
  }),
});
