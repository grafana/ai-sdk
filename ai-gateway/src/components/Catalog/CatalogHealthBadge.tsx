import React from 'react';
import { Badge } from '@grafana/ui';
import { HealthStatus } from '../../api/modelCatalogs';

const labels: Record<Exclude<HealthStatus, 'ok'>, { text: string; color: 'orange' | 'red' }> = {
  invalidKey: { text: 'Key rejected', color: 'red' },
  unknownModel: { text: 'Model not listed', color: 'orange' },
  unreachable: { text: 'Provider unreachable', color: 'orange' },
  refused: { text: 'Listing refused', color: 'orange' },
  notChecked: { text: 'Not checked', color: 'orange' },
};

/** CatalogHealthBadge shows a listing outcome that needs attention. */
export function CatalogHealthBadge({ status }: { status?: HealthStatus }) {
  if (!status || status === 'ok') {
    return null;
  }
  const { text, color } = labels[status];
  return <Badge text={text} color={color} />;
}
