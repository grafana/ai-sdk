import React from 'react';
import { Route, Routes } from 'react-router-dom';
import { AppRootProps } from '@grafana/data';
import { ROUTES } from '../../constants';

const Overview = React.lazy(() => import('../../pages/Overview'));
const CatalogList = React.lazy(() => import('../../pages/CatalogList'));
const CatalogPage = React.lazy(() => import('../../pages/CatalogPage'));

function App(props: AppRootProps) {
  return (
    <Routes>
      <Route path={ROUTES.Catalogs} element={<CatalogList />} />
      <Route path={`${ROUTES.Catalogs}/:name`} element={<CatalogPage />} />
      <Route path="*" element={<Overview />} />
    </Routes>
  );
}

export default App;
