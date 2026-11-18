import React from 'react';
import { Badge } from '@grafana/ui';
import { HealthStatus } from '../../api/modelCatalogs';

const labels: Record<HealthStatus, { text: string; color: 'green' | 'orange' | 'red' | 'blue' }> = {
  ok: { text: 'Model listed', color: 'green' },
  invalidKey: { text: 'Key rejected', color: 'red' },
  unknownModel: { text: 'Model not listed', color: 'orange' },
  unreachable: { text: 'Provider unreachable', color: 'orange' },
  refused: { text: 'Listing refused', color: 'orange' },
  notChecked: { text: 'Not checked', color: 'blue' },
};

/** CatalogHealthBadge shows what a listing did or did not confirm. */
export function CatalogHealthBadge({ status, provider }: { status?: HealthStatus; provider?: boolean }) {
  if (!status) {
    return null;
  }
  const { text, color } = labels[status];
  return <Badge text={provider && status === 'ok' ? 'Listing accepted' : text} color={color} />;
}
