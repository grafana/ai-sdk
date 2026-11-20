import React from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { ClipboardButton, useStyles2 } from '@grafana/ui';

interface Props {
  name: string;
  url: string;
}

/** CatalogURL shows a catalog's base URL with a control to copy it. */
export function CatalogURL({ name, url }: Props) {
  const s = useStyles2(getStyles);

  return (
    <span className={s.container}>
      <code className={s.url}>{url}</code>
      <ClipboardButton
        icon="copy"
        variant="secondary"
        fill="text"
        size="sm"
        aria-label={`Copy the URL of ${name}`}
        getText={() => url}
      />
    </span>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  container: css({
    display: 'inline-flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
    maxWidth: '100%',
  }),
  url: css({
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    fontFamily: theme.typography.fontFamilyMonospace,
    fontSize: theme.typography.sm.fontSize,
    color: theme.colors.text.secondary,
    background: 'none',
    border: 'none',
    padding: 0,
  }),
});
