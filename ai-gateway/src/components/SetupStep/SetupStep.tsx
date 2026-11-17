import React from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Stack, Text, useStyles2 } from '@grafana/ui';

interface Props {
  index: number;
  title: string;
  children: React.ReactNode;
}

export function SetupStep({ index, title, children }: Props) {
  const s = useStyles2(getStyles);

  return (
    <li className={s.step}>
      <span className={s.index} aria-hidden>
        {index}
      </span>
      <Stack direction="column" gap={1} grow={1}>
        <Text element="h3" variant="h5">
          {title}
        </Text>
        {children}
      </Stack>
    </li>
  );
}

const INDEX_SIZE = 4;

const getStyles = (theme: GrafanaTheme2) => ({
  step: css({
    display: 'flex',
    gap: theme.spacing(2),
    minWidth: 0,
  }),
  index: css({
    flexShrink: 0,
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: theme.spacing(INDEX_SIZE),
    height: theme.spacing(INDEX_SIZE),
    borderRadius: theme.shape.radius.circle,
    background: theme.colors.primary.transparent,
    color: theme.colors.primary.text,
    fontWeight: theme.typography.fontWeightMedium,
  }),
});
