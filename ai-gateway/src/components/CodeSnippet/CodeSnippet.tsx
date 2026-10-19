import React from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { ClipboardButton, useStyles2 } from '@grafana/ui';

interface Props {
  code: string;
}

export function CodeSnippet({ code }: Props) {
  const s = useStyles2(getStyles);

  return (
    <div className={s.container}>
      <pre className={s.code}>{code}</pre>
      <div className={s.copy}>
        <ClipboardButton icon="copy" variant="secondary" size="sm" fill="text" getText={() => code}>
          Copy
        </ClipboardButton>
      </div>
    </div>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  container: css({
    position: 'relative',
  }),
  code: css({
    margin: 0,
    padding: theme.spacing(2),
    paddingRight: theme.spacing(10),
    overflowX: 'auto',
    fontFamily: theme.typography.fontFamilyMonospace,
    fontSize: theme.typography.bodySmall.fontSize,
    background: theme.colors.background.secondary,
    border: `1px solid ${theme.colors.border.weak}`,
    borderRadius: theme.shape.radius.default,
  }),
  copy: css({
    position: 'absolute',
    top: theme.spacing(1),
    right: theme.spacing(1),
  }),
});
