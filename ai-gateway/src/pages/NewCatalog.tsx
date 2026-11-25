import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { config, PluginPage } from '@grafana/runtime';
import { Alert, Button, Field, Input, LinkButton, Stack, Text, useStyles2 } from '@grafana/ui';
import { createModelCatalog } from '../api/modelCatalogs';
import { useWrite, WriteError } from '../components/Catalog/FormModal';
import { catalogNameError, keyChange } from '../components/Catalog/catalogSpec';
import { useProviderFields } from '../components/Catalog/useProviderFields';
import { NoCatalogAccess } from '../components/NoCatalogAccess/NoCatalogAccess';
import { catalogBaseURL, catalogPageURL, gatewayEndpoint, prefixRoute, ROUTES } from '../constants';
import { catalogPermissions } from '../permissions';

/** NewCatalog saves the first provider and opens the catalog's page. */
function NewCatalog() {
  const s = useStyles2(getStyles);
  const permissions = catalogPermissions();
  const navigate = useNavigate();
  const [name, setName] = useState('');
  const [submitted, setSubmitted] = useState(false);
  const endpoint = gatewayEndpoint(config.appUrl, config.namespace);
  const provider = useProviderFields({});
  const invalidName = catalogNameError(name);
  const nameError = name !== '' || submitted ? invalidName : undefined;

  const { saving, error, submit } = useWrite(async () => {
    setSubmitted(true);
    const account = provider.submit();
    if (!invalidName && account) {
      await createModelCatalog(
        name,
        { providers: { [account.name]: account.provider }, models: {} },
        keyChange(account.name, account.key)
      );
      navigate(catalogPageURL(name), { state: { checkHealthAfterCreate: true } });
    }
  });

  const body = () => {
    if (!permissions.read) {
      return <NoCatalogAccess />;
    }
    if (!permissions.create) {
      return (
        <Alert severity="info" title="You can't create model catalogs">
          Creating model catalogs needs the Model catalog writer role, which Admins have by default.
        </Alert>
      );
    }
    return (
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <Stack direction="column" gap={2}>
          <Text color="secondary">Connect your first provider now. You can add models after creating the catalog.</Text>
          <Field
            label="Catalog name"
            description="It becomes part of the catalog's URL and can't be changed later. Letters, digits and '-', '_', '.' or ':'."
            invalid={nameError !== undefined}
            error={nameError}
            noMargin
          >
            <Input id="catalog-name" value={name} onChange={(event) => setName(event.currentTarget.value)} />
          </Field>
          {invalidName === undefined && (
            <Stack direction="column" gap={0.5}>
              <Text color="secondary" variant="bodySmall">
                Applications use this URL as their base URL, with a Grafana service account token:
              </Text>
              <span className={s.url}>{catalogBaseURL(endpoint, name)}</span>
            </Stack>
          )}
          {provider.fields}
          <WriteError error={error} />
          <Stack gap={1}>
            <Button type="submit" disabled={saving}>
              {saving ? 'Creating...' : 'Create catalog'}
            </Button>
            <LinkButton variant="secondary" fill="outline" href={prefixRoute(ROUTES.Catalogs)}>
              Cancel
            </LinkButton>
          </Stack>
        </Stack>
      </form>
    );
  };

  return (
    <PluginPage pageNav={{ text: 'New catalog' }}>
      <div className={s.page}>{body()}</div>
    </PluginPage>
  );
}

export default NewCatalog;

const getStyles = (theme: GrafanaTheme2) => ({
  page: css({
    maxWidth: theme.breakpoints.values.lg,
  }),
  url: css({
    fontFamily: theme.typography.fontFamilyMonospace,
    fontSize: theme.typography.sm.fontSize,
    color: theme.colors.text.secondary,
    wordBreak: 'break-all',
  }),
});
