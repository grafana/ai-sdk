import React, { useEffect, useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { config, PluginPage } from '@grafana/runtime';
import {
  Alert,
  Badge,
  Card,
  ClipboardButton,
  EmptyState,
  LinkButton,
  LoadingPlaceholder,
  Stack,
  TextLink,
  useStyles2,
} from '@grafana/ui';
import { errorMessage, listModelCatalogs } from '../api/modelCatalogs';
import { providerTypeLabel } from '../components/Catalog/catalogSpec';
import { NoCatalogAccess } from '../components/NoCatalogAccess/NoCatalogAccess';
import { catalogBaseURL, catalogPageURL, gatewayEndpoint, prefixRoute, ROUTES } from '../constants';
import { ModelCatalog } from '../generated/modelcatalog/v1alpha1/modelcatalog_object_gen';
import { catalogPermissions } from '../permissions';

const DOCS_URL = 'https://github.com/grafana/ai-sdk/blob/main/ai-gateway/README.md#store-model-catalogs-in-grafana';

function count(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? '' : 's'}`;
}

function CatalogList() {
  const s = useStyles2(getStyles);
  const permissions = catalogPermissions();
  const [catalogs, setCatalogs] = useState<ModelCatalog[]>();
  const [error, setError] = useState<string>();
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
  }, [permissions.read]);

  const newCatalog = permissions.create ? (
    <LinkButton icon="plus" href={prefixRoute(ROUTES.NewCatalog)}>
      New catalog
    </LinkButton>
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
          A model catalog gives applications one URL for the models you choose from your own Anthropic or OpenAI
          accounts, with fallbacks between them. Your API keys are stored as Grafana secrets.{' '}
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
              </Card>
            </li>
          );
        })}
      </ul>
    );
  };

  return (
    <PluginPage
      subTitle="A model catalog gives applications one URL for the models you choose from your own Anthropic or OpenAI accounts, with fallbacks between them."
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
