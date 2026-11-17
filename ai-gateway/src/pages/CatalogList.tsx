import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { config, PluginPage } from '@grafana/runtime';
import {
  Alert,
  Badge,
  Button,
  Card,
  ClipboardButton,
  ConfirmModal,
  EmptyState,
  IconButton,
  LoadingPlaceholder,
  Stack,
  TextLink,
  useStyles2,
} from '@grafana/ui';
import { createModelCatalog, deleteModelCatalog, errorMessage, listModelCatalogs } from '../api/modelCatalogs';
import { NewCatalogModal } from '../components/Catalog/NewCatalogModal';
import { providerTypeLabel } from '../components/Catalog/catalogSpec';
import { NoCatalogAccess } from '../components/NoCatalogAccess/NoCatalogAccess';
import { catalogBaseURL, catalogPageURL, gatewayEndpoint } from '../constants';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { catalogPermissions } from '../permissions';

const DOCS_URL = 'https://github.com/grafana/ai-sdk/blob/main/ai-gateway/README.md#store-model-catalogs-in-grafana';

function count(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? '' : 's'}`;
}

function CatalogList() {
  const s = useStyles2(getStyles);
  const permissions = catalogPermissions();
  const navigate = useNavigate();
  const [catalogs, setCatalogs] = useState<ModelCatalog[]>();
  const [error, setError] = useState<string>();
  const [creating, setCreating] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<string>();
  const [loads, setLoads] = useState(0);
  const endpoint = gatewayEndpoint(config.appUrl, config.namespace);

  useEffect(() => {
    if (!permissions.read) {
      return;
    }
    let current = true;
    listModelCatalogs().then(
      (items) => current && setCatalogs(items),
      (err) => current && setError(errorMessage(err))
    );
    return () => {
      current = false;
    };
  }, [permissions.read, loads]);

  const confirmDelete = async () => {
    const name = pendingDelete;
    setPendingDelete(undefined);
    if (name === undefined) {
      return;
    }
    try {
      await deleteModelCatalog(name);
      setError(undefined);
    } catch (err) {
      setError(errorMessage(err));
    }
    setLoads((n) => n + 1);
  };

  const newCatalog = permissions.create ? (
    <Button icon="plus" onClick={() => setCreating(true)}>
      New catalog
    </Button>
  ) : undefined;

  const content = () => {
    if (!permissions.read) {
      return <NoCatalogAccess />;
    }
    if (!catalogs) {
      return error ? null : <LoadingPlaceholder text="Loading catalogs..." />;
    }
    if (catalogs.length === 0) {
      return (
        <EmptyState variant="call-to-action" message="No model catalogs yet" button={newCatalog}>
          A catalog serves models from your own Anthropic or OpenAI accounts, with their API keys stored in Grafana.
          Applications call it at its own URL.{' '}
          <TextLink href={DOCS_URL} external>
            Learn more
          </TextLink>
        </EmptyState>
      );
    }
    return (
      <ul className={s.list}>
        {catalogs.map(({ metadata: { name }, spec }) => {
          const url = catalogBaseURL(endpoint, name);
          const types = [...new Set(Object.values(spec.providers).map((provider) => provider.type))];
          return (
            <li key={name}>
              <Card href={catalogPageURL(name)} noMargin>
                <Card.Heading>{name}</Card.Heading>
                <Card.Meta>
                  {[
                    <span key="providers">{count(Object.keys(spec.providers).length, 'provider')}</span>,
                    <span key="models">{count(Object.keys(spec.models).length, 'model')}</span>,
                  ]}
                </Card.Meta>
                <Card.Description>
                  <span className={s.url}>{url}</span>
                </Card.Description>
                <Card.Tags>
                  <Stack gap={0.5}>
                    {types.map((type) => (
                      <Badge key={type} text={providerTypeLabel(type)} color="blue" />
                    ))}
                  </Stack>
                </Card.Tags>
                <Card.Actions>
                  <ClipboardButton icon="copy" variant="secondary" size="sm" getText={() => url}>
                    Copy URL
                  </ClipboardButton>
                </Card.Actions>
                {permissions.delete && (
                  <Card.SecondaryActions>
                    <IconButton name="trash-alt" tooltip={`Delete ${name}`} onClick={() => setPendingDelete(name)} />
                  </Card.SecondaryActions>
                )}
              </Card>
            </li>
          );
        })}
      </ul>
    );
  };

  return (
    <PluginPage
      subTitle="Serve models from your own provider accounts. Applications call each catalog at its own URL."
      actions={catalogs && catalogs.length > 0 ? newCatalog : undefined}
    >
      <div className={s.page}>
        <Stack direction="column" gap={2}>
          {error && (
            <Alert severity="error" title="Model catalog request failed">
              {error}
            </Alert>
          )}
          {content()}
        </Stack>
      </div>
      {creating && (
        <NewCatalogModal
          endpoint={endpoint}
          onCreate={async (name, spec, secure) => {
            await createModelCatalog(name, spec, secure);
            navigate(catalogPageURL(name));
          }}
          onDismiss={() => setCreating(false)}
        />
      )}
      <ConfirmModal
        isOpen={pendingDelete !== undefined}
        title="Delete model catalog"
        body={`Delete the catalog "${pendingDelete}"? Applications that call its URL stop working, and its saved keys are deleted.`}
        confirmText="Delete"
        onConfirm={confirmDelete}
        onDismiss={() => setPendingDelete(undefined)}
      />
    </PluginPage>
  );
}

export default CatalogList;

const getStyles = (theme: GrafanaTheme2) => ({
  page: css({
    maxWidth: theme.breakpoints.values.lg,
  }),
  list: css({
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(1),
    listStyle: 'none',
    margin: 0,
    padding: 0,
  }),
  url: css({
    fontFamily: theme.typography.fontFamilyMonospace,
    fontSize: theme.typography.sm.fontSize,
  }),
});
