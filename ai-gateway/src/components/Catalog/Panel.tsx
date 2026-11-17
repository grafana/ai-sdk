import React, { ReactNode, useId } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Stack, Text, useStyles2 } from '@grafana/ui';

interface PanelProps {
  title: string;
  description: string;
  action?: ReactNode;
  children: ReactNode;
}

/** Panel is a titled section of a page, with an optional action and a list of rows. */
export function Panel({ title, description, action, children }: PanelProps) {
  const s = useStyles2(getStyles);
  const id = useId();

  return (
    <section className={s.panel} aria-labelledby={id}>
      <div className={s.header}>
        <Stack direction="column" gap={0.5}>
          <Text element="h2" variant="h4" id={id}>
            {title}
          </Text>
          <Text color="secondary" variant="bodySmall">
            {description}
          </Text>
        </Stack>
        {action}
      </div>
      <ul className={s.rows}>{children}</ul>
    </section>
  );
}

interface RowProps {
  label: string;
  actions?: ReactNode;
  children: ReactNode;
}

/** PanelRow is one item of a panel, with its actions at the end. */
export function PanelRow({ label, actions, children }: RowProps) {
  const s = useStyles2(getStyles);

  return (
    <li className={s.row} aria-label={label}>
      <div className={s.content}>{children}</div>
      {actions && <div className={s.actions}>{actions}</div>}
    </li>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  panel: css({
    background: theme.colors.background.primary,
    border: `1px solid ${theme.colors.border.weak}`,
    borderRadius: theme.shape.radius.default,
  }),
  header: css({
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    gap: theme.spacing(2),
    padding: theme.spacing(2),
    borderBottom: `1px solid ${theme.colors.border.weak}`,
  }),
  rows: css({
    listStyle: 'none',
    margin: 0,
    padding: 0,
  }),
  row: css({
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(2),
    padding: theme.spacing(1.5, 2),
    '&:not(:last-child)': {
      borderBottom: `1px solid ${theme.colors.border.weak}`,
    },
  }),
  content: css({
    flex: 1,
    minWidth: 0,
  }),
  actions: css({
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
  }),
});
