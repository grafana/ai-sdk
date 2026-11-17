import React from 'react';
import { Box, Stack, TextLink } from '@grafana/ui';

interface Props {
  title: string;
  href: string;
  children: React.ReactNode;
}

export function LinkCard({ title, href, children }: Props) {
  return (
    <Box borderStyle="solid" borderColor="weak" borderRadius="default" padding={2} backgroundColor="secondary">
      <Stack direction="column" gap={1}>
        <TextLink href={href} external weight="medium">
          {title}
        </TextLink>
        {children}
      </Stack>
    </Box>
  );
}
