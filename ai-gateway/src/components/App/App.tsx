import React from 'react';
import { Route, Routes } from 'react-router-dom';
import { AppRootProps } from '@grafana/data';
import { ROUTES } from '../../constants';

const Overview = React.lazy(() => import('../../pages/Overview'));
const CatalogList = React.lazy(() => import('../../pages/CatalogList'));
const CatalogEdit = React.lazy(() => import('../../pages/CatalogEdit'));

function App(props: AppRootProps) {
  return (
    <Routes>
      <Route path={ROUTES.Catalogs} element={<CatalogList />} />
      <Route path={ROUTES.NewCatalog} element={<CatalogEdit />} />
      <Route path={`${ROUTES.EditCatalog}/:name`} element={<CatalogEdit />} />
      <Route path="*" element={<Overview />} />
    </Routes>
  );
}

export default App;
