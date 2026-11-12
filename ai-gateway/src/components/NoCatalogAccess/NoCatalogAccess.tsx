import React from 'react';
import { Alert } from '@grafana/ui';

/** NoCatalogAccess explains that the user can't read catalogs. */
export function NoCatalogAccess() {
  return (
    <Alert severity="info" title="You can't view model catalogs">
      Reading model catalogs needs the Model catalog reader role, which Viewers, Editors and Admins have by default.
    </Alert>
  );
}
